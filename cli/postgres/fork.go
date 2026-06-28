package postgres

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"pig/cli/ext"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

type Kind string

const (
	KindInstance Kind = "instance"
)

type Mode string

const (
	ModeAuto Mode = "auto"
	ModeHot  Mode = "hot"
	ModeCold Mode = "cold"
)

type BackupMode string

const (
	BackupModeUnknown BackupMode = "unknown"
	BackupModeHot     BackupMode = "hot"
	BackupModeCold    BackupMode = "cold"
)

type CloneMode string

const (
	CloneModeUnknown CloneMode = "unknown"
	CloneModeCOW     CloneMode = "cow"
	CloneModeCopy    CloneMode = "copy"
)

type Options struct {
	Kind     Kind
	Mode     Mode
	DbSU     string
	Plan     bool
	Yes      bool
	Run      bool
	Start    bool
	NoStart  bool
	Replace  bool
	Instance InstanceOptions
}

type InstanceOptions struct {
	Name       string
	SourceData string
	SourcePort int
	DestData   string
	DestPort   int
	Timeout    int
	Managed    bool
}

type State struct {
	BackupMode BackupMode
	CloneMode  CloneMode
	FS         string
	Started    bool
}

type ResultData struct {
	Kind            Kind    `json:"kind" yaml:"kind"`
	Source          string  `json:"source" yaml:"source"`
	Destination     string  `json:"destination" yaml:"destination"`
	SourcePort      int     `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	DestinationPort int     `json:"destination_port,omitempty" yaml:"destination_port,omitempty"`
	BackupMode      string  `json:"backup_mode,omitempty" yaml:"backup_mode,omitempty"`
	CloneMode       string  `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Started         bool    `json:"started" yaml:"started"`
	ConnectCommand  string  `json:"connect_command,omitempty" yaml:"connect_command,omitempty"`
	CleanupCommand  string  `json:"cleanup_command,omitempty" yaml:"cleanup_command,omitempty"`
	Duration        float64 `json:"duration_seconds" yaml:"duration_seconds"`
}

type ForkInfo struct {
	Kind      string         `json:"kind" yaml:"kind"`
	Version   int            `json:"version" yaml:"version"`
	Name      string         `json:"name" yaml:"name"`
	Managed   bool           `json:"managed" yaml:"managed"`
	CreatedAt string         `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Source    ForkEndpoint   `json:"source" yaml:"source"`
	Target    ForkEndpoint   `json:"target" yaml:"target"`
	Copy      ForkCopyInfo   `json:"copy" yaml:"copy"`
	Backup    ForkBackupInfo `json:"backup" yaml:"backup"`
	Commands  ForkCommands   `json:"commands" yaml:"commands"`
	Orphan    bool           `json:"orphan,omitempty" yaml:"orphan,omitempty"`
}

type ForkEndpoint struct {
	Data    string `json:"data" yaml:"data"`
	Port    int    `json:"port,omitempty" yaml:"port,omitempty"`
	Started bool   `json:"started,omitempty" yaml:"started,omitempty"`
}

type ForkCopyInfo struct {
	Method         string `json:"method" yaml:"method"`
	Actual         string `json:"actual" yaml:"actual"`
	Filesystem     string `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	SameFilesystem bool   `json:"same_filesystem,omitempty" yaml:"same_filesystem,omitempty"`
}

type ForkBackupInfo struct {
	Mode  string `json:"mode" yaml:"mode"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
}

type ForkCommands struct {
	Connect string `json:"connect,omitempty" yaml:"connect,omitempty"`
	Stop    string `json:"stop,omitempty" yaml:"stop,omitempty"`
	Remove  string `json:"remove,omitempty" yaml:"remove,omitempty"`
}

type ForkError struct {
	Code int
	Err  error
}

type ForkTargetOptions struct {
	DbSU       string
	Name       string
	DestData   string
	DestPort   int
	Timeout    int
	StopMode   string
	Force      bool
	StopBefore bool
	Yes        bool
}

func (e *ForkError) Error() string {
	if e == nil || e.Err == nil {
		return "fork error"
	}
	return e.Err.Error()
}

func (e *ForkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var forkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

var forkDBSUCommand = utils.DBSUCommand
var forkDBSUCommandOutput = utils.DBSUCommandOutput
var forkCheckDataDir = CheckDataDirAsDBSU
var forkCheckPostgresRunning = CheckPostgresRunningAsDBSU
var forkReadFileAsDBSU = utils.ReadFileAsDBSU
var forkWriteFileAsDBSU = utils.WriteFileAsDBSU
var forkPortFree = isPortFree
var forkXFSInfoOutput = func(bin, mount string) ([]byte, error) {
	return exec.Command(bin, mount).Output()
}

func NormalizeOptions(opts *Options) (*Options, error) {
	if opts == nil {
		return nil, fmt.Errorf("fork options are required")
	}
	n := *opts
	n.DbSU = utils.GetDBSU(n.DbSU)
	if n.Mode == "" {
		n.Mode = ModeAuto
	}
	if n.Mode != ModeAuto && n.Mode != ModeHot && n.Mode != ModeCold {
		return nil, fmt.Errorf("invalid fork mode %q (valid: auto, hot, cold)", n.Mode)
	}
	n.Start = n.Run && !n.NoStart

	switch n.Kind {
	case KindInstance:
		if err := normalizeInstance(&n); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid fork kind %q (valid: instance)", n.Kind)
	}
	return &n, nil
}

func normalizeInstance(opts *Options) error {
	inst := &opts.Instance
	inst.Name = strings.TrimSpace(inst.Name)
	if inst.Name == "" {
		return fmt.Errorf("fork name is required")
	}
	if !forkNamePattern.MatchString(inst.Name) {
		return fmt.Errorf("invalid fork name %q (use letters, numbers, dot, underscore, or dash)", inst.Name)
	}
	if inst.SourceData == "" {
		inst.SourceData = os.Getenv("PG_DATA")
	}
	if inst.SourceData == "" {
		inst.SourceData = "/pg/data"
	}
	if inst.SourcePort == 0 {
		if port := os.Getenv("PG_PORT"); port != "" {
			if p, err := strconv.Atoi(port); err == nil && p > 0 {
				inst.SourcePort = p
			}
		}
	}
	if inst.SourcePort == 0 {
		inst.SourcePort = 5432
	}
	if !validPort(inst.SourcePort) {
		return fmt.Errorf("invalid source port %d (must be 1-65535)", inst.SourcePort)
	}
	if inst.DestData == "" {
		dest, err := ManagedForkDataDir(inst.Name)
		if err != nil {
			return err
		}
		inst.DestData = dest
		inst.Managed = true
	} else {
		inst.Managed = false
	}
	if inst.DestPort == 0 {
		inst.DestPort = firstFreePortAvoiding(15432, reservedManagedForkPorts(opts.DbSU, inst.DestData))
	}
	if !validPort(inst.DestPort) {
		return fmt.Errorf("invalid destination port %d (must be 1-65535)", inst.DestPort)
	}
	if inst.Timeout == 0 {
		inst.Timeout = 60
	}
	return nil
}

func Plan(opts *Options) (*output.Plan, error) {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	if n.Kind == KindInstance {
		sourceData, destData, err := validateForkDataPaths(n.Instance.SourceData, n.Instance.DestData)
		if err != nil {
			return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
		}
		n.Instance.SourceData = sourceData
		n.Instance.DestData = destData
	}
	return BuildPlan(n, inferPlanState(n)), nil
}

func Execute(opts *Options) error {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return exitForkError(output.CodeForkInvalidArgs, err)
	}
	if !n.Yes {
		if err := confirmForkWithCountdown("This will create a PostgreSQL instance fork and may replace the destination!", "FORK"); err != nil {
			return exitForkError(output.CodeForkInvalidArgs, err)
		}
	}
	_, err = executeNormalized(n)
	if err != nil {
		if fe, ok := err.(*ForkError); ok {
			return &utils.ExitCodeError{Code: output.ExitCode(fe.Code), Err: fe}
		}
		return err
	}
	return nil
}

func ExecuteResult(opts *Options) *output.Result {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return output.Fail(output.CodeForkInvalidArgs, err.Error())
	}
	data, err := executeNormalized(n)
	if err != nil {
		if fe, ok := err.(*ForkError); ok {
			return output.Fail(fe.Code, fe.Error())
		}
		return output.Fail(output.CodeForkPrecheckFailed, err.Error())
	}
	return output.OK("instance fork completed", data)
}

func BuildPlan(opts *Options, state *State) *output.Plan {
	if opts == nil {
		return &output.Plan{Command: "pig pg fork"}
	}
	switch opts.Kind {
	case KindInstance:
		return buildInstancePlan(opts, state)
	default:
		return &output.Plan{Command: BuildCommand(opts)}
	}
}

func buildInstancePlan(opts *Options, state *State) *output.Plan {
	inst := opts.Instance
	backupMode := BackupModeHot
	cloneMode := CloneModeCopy
	if state != nil {
		if state.BackupMode != "" && state.BackupMode != BackupModeUnknown {
			backupMode = state.BackupMode
		}
		if state.CloneMode != "" && state.CloneMode != CloneModeUnknown {
			cloneMode = state.CloneMode
		}
	}

	actions := []output.Action{}
	step := 1
	if backupMode == BackupModeHot {
		actions = append(actions, output.Action{Step: step, Description: "Start PostgreSQL backup mode"})
		step++
	} else {
		actions = append(actions, output.Action{Step: step, Description: "Use cold copy mode"})
		step++
	}
	copyDesc := "Clone data directory"
	if cloneMode == CloneModeCOW {
		copyDesc = "Clone data directory with CoW"
	}
	actions = append(actions,
		output.Action{Step: step, Description: copyDesc},
		output.Action{Step: step + 1, Description: "Prepare forked instance configuration"},
	)
	step += 2
	if opts.Start {
		actions = append(actions, output.Action{Step: step, Description: "Start forked PostgreSQL instance"})
		step++
		actions = append(actions, output.Action{Step: step, Description: "Verify forked instance is reachable"})
	}

	risks := []string{"Destination data directory will be removed when --force is used"}
	if cloneMode == CloneModeCOW {
		risks = append(risks, "Copy-on-write forks share physical blocks until either side writes")
	} else {
		risks = append(risks, "Execution requires verified CoW support; use --force to allow regular copy fallback")
	}
	if backupMode == BackupModeCold {
		risks = append(risks, "Cold copy requires the source instance to be stopped")
	}

	return &output.Plan{
		Command: BuildCommand(opts),
		Actions: actions,
		Affects: []output.Resource{
			{Type: "instance", Name: inst.SourceData, Impact: "read", Detail: fmt.Sprintf("port %d", inst.SourcePort)},
			{Type: "instance", Name: inst.DestData, Impact: "create", Detail: fmt.Sprintf("port %d", inst.DestPort)},
		},
		Expected: fmt.Sprintf("PostgreSQL instance forked from %s to %s on port %d", inst.SourceData, inst.DestData, inst.DestPort),
		Risks:    risks,
	}
}

func BuildCommand(opts *Options) string {
	if opts == nil {
		return "pig pg fork"
	}
	args := []string{"pig", "pg"}
	switch opts.Kind {
	case KindInstance:
		args = append(args, "fork", "init", opts.Instance.Name)
		if opts.Instance.SourceData != "" && opts.Instance.SourceData != "/pg/data" {
			args = append(args, "-D", quoteArg(opts.Instance.SourceData))
		}
		if opts.Instance.SourcePort != 0 && opts.Instance.SourcePort != 5432 {
			args = append(args, "-P", fmt.Sprintf("%d", opts.Instance.SourcePort))
		}
		if opts.Instance.DestData != "" && !opts.Instance.Managed {
			args = append(args, "-d", quoteArg(opts.Instance.DestData))
		}
		if opts.Instance.DestPort != 0 && opts.Instance.DestPort != 15432 {
			args = append(args, "-p", fmt.Sprintf("%d", opts.Instance.DestPort))
		}
		if opts.Run {
			args = append(args, "-r")
		}
		if opts.Replace {
			args = append(args, "-f")
		}
	}
	if opts.Yes {
		args = append(args, "-y")
	}
	if opts.Plan {
		args = append(args, "--plan")
	}
	return strings.Join(args, " ")
}

func BuildForkInfo(opts *Options, state *State) ForkInfo {
	inst := opts.Instance
	info := ForkInfo{
		Kind:      "pg_fork",
		Version:   1,
		Name:      inst.Name,
		Managed:   inst.Managed,
		CreatedAt: time.Now().Format(time.RFC3339),
		Source: ForkEndpoint{
			Data: inst.SourceData,
			Port: inst.SourcePort,
		},
		Target: ForkEndpoint{
			Data:    inst.DestData,
			Port:    inst.DestPort,
			Started: opts.Start,
		},
		Copy: ForkCopyInfo{
			Method: "reflink_auto",
			Actual: string(CloneModeUnknown),
		},
		Backup: ForkBackupInfo{
			Mode: string(BackupModeUnknown),
		},
		Commands: ForkCommands{
			Connect: utils.ShellQuoteArgs([]string{"psql", "-p", strconv.Itoa(inst.DestPort)}),
			Stop:    utils.ShellQuoteArgs([]string{"pg_ctl", "-D", inst.DestData, "stop"}),
			Remove:  utils.ShellQuoteArgs([]string{"rm", "-rf", "--", inst.DestData}),
		},
	}
	if state != nil {
		info.Copy.Actual = string(state.CloneMode)
		info.Copy.Filesystem = state.FS
		info.Backup.Mode = string(state.BackupMode)
		info.Target.Started = state.Started
	}
	return info
}

func WriteForkInfoAs(dbsu, dataDir string, info ForkInfo) error {
	payload, err := marshalForkInfo(info)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "pig-fork-info-*.json")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(path, 0644); err != nil {
		return err
	}
	dest := filepath.Join(dataDir, "fork.json")
	if err := utils.DBSUCommand(dbsu, []string{"cp", path, dest}); err != nil {
		return err
	}
	return utils.DBSUCommand(dbsu, []string{"chmod", "0644", dest})
}

func marshalForkInfo(info ForkInfo) ([]byte, error) {
	payload, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	return payload, nil
}

func ScanForks(root string) ([]ForkInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []ForkInfo{}, nil
		}
		return nil, err
	}
	forks := make([]ForkInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "data-") {
			continue
		}
		dataDir := filepath.Join(root, entry.Name())
		info, err := readForkInfo(dataDir)
		if err != nil {
			name := strings.TrimPrefix(entry.Name(), "data-")
			info = ForkInfo{
				Kind:    "pg_fork",
				Version: 1,
				Name:    name,
				Managed: true,
				Target:  ForkEndpoint{Data: dataDir},
				Orphan:  true,
			}
		}
		forks = append(forks, info)
	}
	sort.Slice(forks, func(i, j int) bool { return forks[i].Name < forks[j].Name })
	return forks, nil
}

func ScanForksAs(dbsu, root string) ([]ForkInfo, error) {
	dbsu = utils.GetDBSU(dbsu)
	out, err := forkDBSUCommandOutput(dbsu, []string{"find", "-H", root, "-mindepth", "1", "-maxdepth", "1", "-type", "d", "-name", "data-*", "-print"})
	if err != nil {
		if strings.Contains(out, "No such file") || strings.Contains(out, "no such file") {
			return []ForkInfo{}, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	forks := make([]ForkInfo, 0, len(lines))
	for _, line := range lines {
		dataDir := strings.TrimSpace(line)
		if dataDir == "" {
			continue
		}
		info, err := ReadForkInfoAs(dbsu, dataDir)
		if err != nil {
			name := strings.TrimPrefix(filepath.Base(dataDir), "data-")
			info = ForkInfo{
				Kind:    "pg_fork",
				Version: 1,
				Name:    name,
				Managed: true,
				Target:  ForkEndpoint{Data: dataDir},
				Orphan:  true,
			}
		}
		forks = append(forks, info)
	}
	sort.Slice(forks, func(i, j int) bool { return forks[i].Name < forks[j].Name })
	return forks, nil
}

func readForkInfo(dataDir string) (ForkInfo, error) {
	payload, err := os.ReadFile(filepath.Join(dataDir, "fork.json"))
	if err != nil {
		return ForkInfo{}, err
	}
	var info ForkInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return ForkInfo{}, err
	}
	if info.Target.Data == "" {
		info.Target.Data = dataDir
	}
	if info.Name == "" {
		info.Name = strings.TrimPrefix(filepath.Base(dataDir), "data-")
	}
	if !info.Managed {
		if managedDataDir, err := ManagedForkDataDir(info.Name); err == nil && filepath.Clean(info.Target.Data) == managedDataDir {
			info.Managed = true
		}
	}
	return info, nil
}

func ManagedForkDataDir(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("fork name is required")
	}
	if !forkNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid fork name %q (use letters, numbers, dot, underscore, or dash)", name)
	}
	return "/pg/data-" + name, nil
}

func ResolveForkTarget(opts ForkTargetOptions) (string, error) {
	name := strings.TrimSpace(opts.Name)
	dest := strings.TrimSpace(opts.DestData)
	if name == "" && dest == "" {
		return "", fmt.Errorf("fork name or --dst-data is required")
	}
	if name != "" && dest != "" {
		return "", fmt.Errorf("fork name and --dst-data are mutually exclusive")
	}
	if dest != "" {
		return cleanAbsPath(dest)
	}
	return ManagedForkDataDir(name)
}

func ReadForkInfoAs(dbsu, dataDir string) (ForkInfo, error) {
	payload, err := forkReadFileAsDBSU(filepath.Join(dataDir, "fork.json"), dbsu)
	if err != nil {
		return ForkInfo{}, err
	}
	var info ForkInfo
	if err := json.Unmarshal([]byte(payload), &info); err != nil {
		return ForkInfo{}, err
	}
	completeForkInfoDefaults(&info, dataDir)
	return info, nil
}

func completeForkInfoDefaults(info *ForkInfo, dataDir string) {
	if info.Target.Data == "" {
		info.Target.Data = dataDir
	}
	if info.Name == "" {
		info.Name = strings.TrimPrefix(filepath.Base(dataDir), "data-")
	}
	if !info.Managed {
		if managedDataDir, err := ManagedForkDataDir(info.Name); err == nil && filepath.Clean(info.Target.Data) == managedDataDir {
			info.Managed = true
		}
	}
}

func StartFork(opts ForkTargetOptions) (ResultData, error) {
	dbsu := utils.GetDBSU(opts.DbSU)
	dataDir, err := ResolveForkTarget(opts)
	if err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	info, err := requireForkInfo(dbsu, dataDir)
	if err != nil {
		return ResultData{}, err
	}
	port := opts.DestPort
	if port == 0 {
		port = info.Target.Port
	}
	if port == 0 {
		port = firstFreePortAvoiding(15432, reservedManagedForkPorts(dbsu, dataDir))
	}
	if !validPort(port) {
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: fmt.Errorf("invalid destination port %d (must be 1-65535)", port)}
	}
	if running, _ := CheckPostgresRunningAsDBSU(dbsu, dataDir); running {
		info.Target.Started = true
		_ = WriteForkInfoAs(dbsu, dataDir, info)
		return ResultData{Kind: KindInstance, Destination: dataDir, DestinationPort: port, Started: true, ConnectCommand: utils.ShellQuoteArgs([]string{"psql", "-p", strconv.Itoa(port)})}, nil
	}
	if forkPortReservedByManagedFork(dbsu, port, dataDir) {
		return ResultData{}, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is reserved by another managed fork: %d", port)}
	}
	if !forkPortFree(port) {
		return ResultData{}, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is in use: %d", port)}
	}
	if opts.DestPort != 0 {
		if err := configureInstance(dbsu, dataDir, port); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkConfigFailed, Err: err}
		}
	}
	cfg := &Config{PgData: dataDir, DbSU: dbsu}
	if err := Start(cfg, &StartOptions{Timeout: opts.Timeout, LogFile: filepath.Join(dataDir, "log", "fork.log")}); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkStartFailed, Err: err}
	}
	if err := verifyInstance(dbsu, port); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkVerifyFailed, Err: err}
	}
	info.Target.Port = port
	info.Target.Started = true
	_ = WriteForkInfoAs(dbsu, dataDir, info)
	return ResultData{Kind: KindInstance, Destination: dataDir, DestinationPort: port, Started: true, ConnectCommand: utils.ShellQuoteArgs([]string{"psql", "-p", strconv.Itoa(port)})}, nil
}

func StopFork(opts ForkTargetOptions) (ResultData, error) {
	dbsu := utils.GetDBSU(opts.DbSU)
	dataDir, err := ResolveForkTarget(opts)
	if err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	info, err := requireForkInfo(dbsu, dataDir)
	if err != nil {
		return ResultData{}, err
	}
	if running, _ := CheckPostgresRunningAsDBSU(dbsu, dataDir); !running {
		info.Target.Started = false
		_ = WriteForkInfoAs(dbsu, dataDir, info)
		return ResultData{Kind: KindInstance, Destination: dataDir, DestinationPort: info.Target.Port, Started: false}, nil
	}
	cfg := &Config{PgData: dataDir, DbSU: dbsu}
	if err := Stop(cfg, &StopOptions{Mode: opts.StopMode, Timeout: opts.Timeout}); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkStopFailed, Err: err}
	}
	info.Target.Started = false
	_ = WriteForkInfoAs(dbsu, dataDir, info)
	return ResultData{Kind: KindInstance, Destination: dataDir, DestinationPort: info.Target.Port, Started: false}, nil
}

func RemoveFork(opts ForkTargetOptions) (ResultData, error) {
	dbsu := utils.GetDBSU(opts.DbSU)
	dataDir, err := ResolveForkTarget(opts)
	if err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	info, err := requireForkInfo(dbsu, dataDir)
	if err != nil {
		orphan, ok := forcedManagedOrphanInfo(dbsu, opts, dataDir)
		if !ok {
			return ResultData{}, err
		}
		info = orphan
	}
	running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)
	if running {
		if !opts.StopBefore || !opts.Force {
			return ResultData{}, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("fork is running (PID: %d): %s; use --stop -f to stop and remove it", pid, dataDir)}
		}
		cfg := &Config{PgData: dataDir, DbSU: dbsu}
		if err := Stop(cfg, &StopOptions{Mode: opts.StopMode, Timeout: opts.Timeout}); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkStopFailed, Err: err}
		}
	}
	if !opts.Yes && !opts.Force {
		if err := confirmForkWithCountdown(fmt.Sprintf("This will remove PostgreSQL fork %s", dataDir), "REMOVE"); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
		}
	}
	if err := removeForkDataDir(dbsu, dataDir); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkRemoveFailed, Err: err}
	}
	return ResultData{Kind: KindInstance, Destination: dataDir, DestinationPort: info.Target.Port, Started: false}, nil
}

func requireForkInfo(dbsu, dataDir string) (ForkInfo, error) {
	if err := validateForkRemovalPath(dataDir); err != nil {
		return ForkInfo{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	exists, initialized := forkCheckDataDir(dbsu, dataDir)
	if !exists || !initialized {
		return ForkInfo{}, &ForkError{Code: output.CodeForkSourceNotFound, Err: fmt.Errorf("fork data directory is not initialized: %s", dataDir)}
	}
	info, err := ReadForkInfoAs(dbsu, dataDir)
	if err != nil {
		return ForkInfo{}, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("fork metadata not found: %s/fork.json", dataDir)}
	}
	if info.Kind != "pg_fork" {
		return ForkInfo{}, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("invalid fork kind %q in %s/fork.json", info.Kind, dataDir)}
	}
	return info, nil
}

func forcedManagedOrphanInfo(dbsu string, opts ForkTargetOptions, dataDir string) (ForkInfo, bool) {
	if !opts.Force || opts.Name == "" || opts.DestData != "" {
		return ForkInfo{}, false
	}
	managedDataDir, err := ManagedForkDataDir(opts.Name)
	if err != nil || managedDataDir != dataDir {
		return ForkInfo{}, false
	}
	exists, _ := forkCheckDataDir(dbsu, dataDir)
	if !exists {
		return ForkInfo{}, false
	}
	return ForkInfo{
		Kind:    "pg_fork",
		Version: 1,
		Name:    opts.Name,
		Managed: true,
		Target:  ForkEndpoint{Data: dataDir},
		Orphan:  true,
	}, true
}

func removeForkDataDir(dbsu, dataDir string) error {
	if err := validateForkRemovalPath(dataDir); err != nil {
		return err
	}
	args := []string{"rm", "-rf", "--", dataDir}
	fmt.Fprintf(os.Stderr, "%s$ %s%s\n", utils.ColorBlue, utils.ShellQuoteArgs(args), utils.ColorReset)
	return forkDBSUCommand(dbsu, args)
}

func validateForkRemovalPath(dataDir string) error {
	path, err := cleanAbsPath(dataDir)
	if err != nil {
		return err
	}
	if path == "/" || path == "/pg" || path == "/var" || path == "/tmp" {
		return fmt.Errorf("unsafe fork data directory: %s", dataDir)
	}
	return nil
}

func executeNormalized(opts *Options) (ResultData, error) {
	start := time.Now()
	switch opts.Kind {
	case KindInstance:
		state, err := precheckInstance(opts)
		if err != nil {
			return ResultData{}, err
		}
		if err := executeInstance(opts, state); err != nil {
			return ResultData{}, err
		}
		return instanceResult(opts, state, time.Since(start)), nil
	default:
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: fmt.Errorf("invalid fork kind %q", opts.Kind)}
	}
}

func exitForkError(code int, err error) error {
	fe := &ForkError{Code: code, Err: err}
	return &utils.ExitCodeError{Code: output.ExitCode(code), Err: fe}
}

func confirmForkWithCountdown(warning, action string) error {
	fmt.Fprintf(os.Stderr, "\n%sWARNING: %s%s\n", utils.ColorYellow, warning, utils.ColorReset)
	fmt.Fprintln(os.Stderr, "Press Ctrl+C to cancel, or wait for countdown...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigChan)
		close(sigChan)
	}()

	for i := 5; i > 0; i-- {
		select {
		case <-sigChan:
			fmt.Fprintf(os.Stderr, "\n%s cancelled.\n", action)
			return fmt.Errorf("%s cancelled by user", action)
		case <-time.After(time.Second):
			fmt.Fprintf(os.Stderr, "\rStarting %s in %d seconds... ", action, i)
		}
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func inferPlanState(opts *Options) *State {
	state := &State{CloneMode: CloneModeUnknown}
	if opts == nil {
		return state
	}
	switch opts.Mode {
	case ModeCold:
		state.BackupMode = BackupModeCold
	default:
		state.BackupMode = BackupModeHot
	}
	return state
}

func precheckInstance(opts *Options) (*State, error) {
	inst := opts.Instance
	sourceData, destData, err := validateForkDataPaths(inst.SourceData, inst.DestData)
	if err != nil {
		return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	opts.Instance.SourceData = sourceData
	opts.Instance.DestData = destData
	inst = opts.Instance

	exists, initialized := CheckDataDirAsDBSU(opts.DbSU, inst.SourceData)
	if !exists || !initialized {
		return nil, &ForkError{Code: output.CodeForkSourceNotFound, Err: fmt.Errorf("source data directory is not initialized: %s", inst.SourceData)}
	}

	if destExists, _ := CheckDataDirAsDBSU(opts.DbSU, inst.DestData); destExists {
		if running, pid := CheckPostgresRunningAsDBSU(opts.DbSU, inst.DestData); running {
			return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("destination fork is running (PID: %d): %s; stop it before replacing", pid, inst.DestData)}
		}
		if !opts.Replace {
			return nil, &ForkError{Code: output.CodeForkDestExists, Err: fmt.Errorf("destination data directory exists: %s (use --replace)", inst.DestData)}
		}
	}

	if forkPortReservedByManagedFork(opts.DbSU, inst.DestPort, inst.DestData) {
		return nil, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is reserved by another managed fork: %d", inst.DestPort)}
	}
	if !forkPortFree(inst.DestPort) {
		return nil, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is in use: %d", inst.DestPort)}
	}

	running := canConnect(opts.DbSU, inst.SourcePort)
	if opts.Mode == ModeHot && !running {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source instance is not reachable on port %d; use --mode cold if it is stopped", inst.SourcePort)}
	}
	if (opts.Mode == ModeCold || (opts.Mode == ModeAuto && !running)) && hasPostmasterPID(opts.DbSU, inst.SourceData) {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source data directory has postmaster.pid; refusing cold copy of a possibly running instance")}
	}

	mode := BackupModeHot
	if opts.Mode == ModeCold || (opts.Mode == ModeAuto && !running) {
		mode = BackupModeCold
	}
	cloneMode, fs := detectCloneModeAs(opts.DbSU, inst.SourceData, inst.DestData)
	state := &State{BackupMode: mode, CloneMode: cloneMode, FS: fs}
	if err := requireCOW(state, opts.Replace); err != nil {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	return state, nil
}

func executeInstance(opts *Options, state *State) error {
	inst := opts.Instance
	if state.BackupMode == BackupModeCold {
		if err := coldCopy(opts.DbSU, inst.SourceData, inst.DestData); err != nil {
			return &ForkError{Code: output.CodeForkCopyFailed, Err: err}
		}
	} else {
		if err := hotCopy(opts.DbSU, inst); err != nil {
			return err
		}
	}

	if err := configureInstance(opts.DbSU, inst.DestData, inst.DestPort); err != nil {
		return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}

	if opts.Start {
		cfg := &Config{PgData: inst.DestData, DbSU: opts.DbSU}
		if err := Start(cfg, forkStartOptions(inst)); err != nil {
			return &ForkError{Code: output.CodeForkStartFailed, Err: err}
		}
		if err := verifyInstance(opts.DbSU, inst.DestPort); err != nil {
			return &ForkError{Code: output.CodeForkVerifyFailed, Err: err}
		}
		state.Started = true
	}
	info := BuildForkInfo(opts, state)
	if err := WriteForkInfoAs(opts.DbSU, inst.DestData, info); err != nil {
		return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}
	return nil
}

func forkStartOptions(inst InstanceOptions) *StartOptions {
	return &StartOptions{
		Timeout: inst.Timeout,
		LogFile: filepath.Join(inst.DestData, "log", "fork.log"),
	}
}

func coldCopy(dbsu, src, dst string) error {
	return copyDataDir(dbsu, src, dst)
}

func hotCopy(dbsu string, inst InstanceOptions) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return &ForkError{Code: output.CodeForkDependencyMissing, Err: fmt.Errorf("postgresql not found: %w", err)}
	}
	session, err := newPsqlBackupSession(dbsu, pg.Psql(), inst.SourcePort)
	if err != nil {
		return &ForkError{Code: output.CodeForkBackupFailed, Err: err}
	}
	defer session.Close()

	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	label := fmt.Sprintf("pig_fork_%s_%s", inst.Name, time.Now().Format("20060102_150405"))
	copyFn := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyDataDir(dbsu, inst.SourceData, inst.DestData); err != nil {
			return err
		}
		return ctx.Err()
	}
	if err := runHotBackupCopy(session, label, copyFn); err != nil {
		var copyErr copyPhaseError
		if errors.As(err, &copyErr) {
			return &ForkError{Code: output.CodeForkCopyFailed, Err: copyErr.Err}
		}
		return &ForkError{Code: output.CodeForkBackupFailed, Err: err}
	}
	if err := validateCopiedDataDir(dbsu, inst.DestData); err != nil {
		return &ForkError{Code: output.CodeForkCopyFailed, Err: err}
	}
	return nil
}

func copyDataDir(dbsu, src, dst string) error {
	if running, pid := forkCheckPostgresRunning(dbsu, dst); running {
		return fmt.Errorf("destination data directory is running (PID: %d): %s", pid, dst)
	}
	commands := [][]string{
		{"rm", "-rf", "--", dst},
		{"cp", "-a", "--reflink=auto", src, dst},
		{"test", "-f", filepath.Join(dst, "PG_VERSION")},
	}
	for _, args := range commands {
		if err := forkDBSUCommand(dbsu, args); err != nil {
			return err
		}
	}
	return nil
}

func validateCopiedDataDir(dbsu, dst string) error {
	return utils.DBSUCommand(dbsu, []string{"test", "-f", filepath.Join(dst, "PG_VERSION")})
}

type backupSession interface {
	Exec(sql string) (string, error)
	Close() error
}

type backupFunctions struct {
	start  string
	stop   string
	legacy bool
}

type copyPhaseError struct {
	Err error
}

func (e copyPhaseError) Error() string {
	return e.Err.Error()
}

func (e copyPhaseError) Unwrap() error {
	return e.Err
}

func runHotBackupCopy(session backupSession, label string, copyFn func() error) error {
	version, err := backupServerVersion(session)
	if err != nil {
		return err
	}
	names := backupFunctionNames(version)
	if _, err := session.Exec(buildBackupStartSQL(label, names)); err != nil {
		return err
	}
	copyErr := copyFn()
	_, stopErr := session.Exec(buildBackupStopSQL(names))
	if copyErr != nil {
		if stopErr != nil {
			return copyPhaseError{Err: fmt.Errorf("copy failed: %w; backup stop also failed: %v", copyErr, stopErr)}
		}
		return copyPhaseError{Err: copyErr}
	}
	return stopErr
}

func backupServerVersion(session backupSession) (int, error) {
	out, err := session.Exec("SELECT current_setting('server_version_num');")
	if err != nil {
		return 0, err
	}
	version, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("invalid server_version_num %q: %w", strings.TrimSpace(out), err)
	}
	return version, nil
}

func backupFunctionNames(version int) backupFunctions {
	if version < 150000 {
		return backupFunctions{start: "pg_start_backup", stop: "pg_stop_backup", legacy: true}
	}
	return backupFunctions{start: "pg_backup_start", stop: "pg_backup_stop"}
}

func buildBackupStartSQL(label string, names backupFunctions) string {
	if names.legacy {
		return fmt.Sprintf("CHECKPOINT;\nSELECT %s('%s', true, false);\n", names.start, EscapeSQLString(label))
	}
	return fmt.Sprintf("CHECKPOINT;\nSELECT %s('%s', fast => true);\n", names.start, EscapeSQLString(label))
}

func buildBackupStopSQL(names backupFunctions) string {
	if names.legacy {
		return fmt.Sprintf("SELECT * FROM %s(false, false);\n", names.stop)
	}
	return fmt.Sprintf("SELECT * FROM %s(wait_for_archive => false);\n", names.stop)
}

type psqlBackupSession struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	out     *bufio.Scanner
	errbuf  *bytes.Buffer
	seq     int
	waited  bool
	waitErr error
}

func newPsqlBackupSession(dbsu, psql string, port int) (*psqlBackupSession, error) {
	args := []string{psql, "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-p", fmt.Sprintf("%d", port), "-d", "postgres"}
	cmd, err := utils.BuildDBSUCommand(dbsu, args)
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errbuf := &bytes.Buffer{}
	cmd.Stderr = errbuf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	return &psqlBackupSession{cmd: cmd, stdin: stdin, out: scanner, errbuf: errbuf}, nil
}

func (s *psqlBackupSession) Exec(sql string) (string, error) {
	s.seq++
	marker := fmt.Sprintf("__PIG_FORK_SQL_DONE_%d__", s.seq)
	if _, err := fmt.Fprintf(s.stdin, "%s\n\\echo %s\n", strings.TrimSpace(sql), marker); err != nil {
		return "", err
	}
	lines := []string{}
	for s.out.Scan() {
		line := strings.TrimSpace(s.out.Text())
		if line == marker {
			return strings.TrimSpace(strings.Join(lines, "\n")), nil
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := s.out.Err(); err != nil {
		return strings.TrimSpace(strings.Join(lines, "\n")), err
	}
	waitErr := s.wait()
	errText := strings.TrimSpace(s.errbuf.String())
	if waitErr != nil {
		if errText != "" {
			return strings.TrimSpace(strings.Join(lines, "\n")), fmt.Errorf("%w: %s", waitErr, errText)
		}
		return strings.TrimSpace(strings.Join(lines, "\n")), waitErr
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), fmt.Errorf("psql session ended before marker %s", marker)
}

func (s *psqlBackupSession) Close() error {
	if s == nil {
		return nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	return s.wait()
}

func (s *psqlBackupSession) wait() error {
	if s.waited {
		return s.waitErr
	}
	s.waitErr = s.cmd.Wait()
	s.waited = true
	return s.waitErr
}

func configureInstance(dbsu, dataDir string, port int) error {
	runtimeFiles := []string{
		filepath.Join(dataDir, "postmaster.pid"),
		filepath.Join(dataDir, "postmaster.opts"),
		filepath.Join(dataDir, "standby.signal"),
		filepath.Join(dataDir, "recovery.signal"),
	}
	if err := forkDBSUCommand(dbsu, append([]string{"rm", "-f"}, runtimeFiles...)); err != nil {
		return err
	}
	replslot := filepath.Join(dataDir, "pg_replslot")
	if err := forkDBSUCommand(dbsu, []string{"rm", "-rf", replslot}); err != nil {
		return err
	}
	if err := forkDBSUCommand(dbsu, []string{"mkdir", "-p", replslot}); err != nil {
		return err
	}
	autoconf := filepath.Join(dataDir, "postgresql.auto.conf")
	if err := forkDBSUCommand(dbsu, []string{"touch", autoconf}); err != nil {
		return err
	}
	content, err := forkReadFileAsDBSU(autoconf, dbsu)
	if err != nil {
		return err
	}
	return forkWriteFileAsDBSU(autoconf, rewriteForkAutoConf(content, port), dbsu)
}

func rewriteForkAutoConf(content string, port int) string {
	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "port = "):
			continue
		case strings.HasPrefix(trimmed, "archive_mode = "):
			continue
		case strings.HasPrefix(trimmed, "log_directory = "):
			continue
		case strings.HasPrefix(trimmed, "primary_conninfo"):
			continue
		case strings.HasPrefix(trimmed, "primary_slot_name"):
			continue
		case strings.HasPrefix(trimmed, "recovery_target"):
			continue
		default:
			lines = append(lines, line)
		}
	}
	lines = append(lines,
		fmt.Sprintf("port = %d", port),
		"archive_mode = off",
		"log_directory = 'log'",
	)
	return strings.Join(lines, "\n") + "\n"
}

func verifyInstance(dbsu string, port int) error {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return err
	}
	_, err = utils.DBSUCommandOutput(dbsu, forkPsqlProbeArgs(pg.Psql(), port))
	return err
}

func canConnect(dbsu string, port int) bool {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return false
	}
	_, err = utils.DBSUCommandOutput(dbsu, forkPsqlProbeArgs(pg.Psql(), port))
	return err == nil
}

func forkPsqlProbeArgs(psql string, port int) []string {
	return []string{psql, "-X", "-p", fmt.Sprintf("%d", port), "-d", "postgres", "-Atc", "SELECT 1"}
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func firstFreePort(start int) int {
	return firstFreePortAvoiding(start, nil)
}

func firstFreePortAvoiding(start int, reserved map[int]bool) int {
	for port := start; port < start+1000 && port <= 65535; port++ {
		if reserved[port] {
			continue
		}
		if forkPortFree(port) {
			return port
		}
	}
	return start
}

func reservedManagedForkPorts(dbsu, excludeDataDir string) map[int]bool {
	if _, err := os.Stat("/pg"); err != nil {
		return nil
	}
	forks, err := ScanForksAs(dbsu, "/pg")
	if err != nil {
		logrus.Debugf("scan managed forks for port reservation failed: %v", err)
		return nil
	}
	return reservedForkPorts(forks, excludeDataDir)
}

func reservedForkPorts(forks []ForkInfo, excludeDataDir string) map[int]bool {
	reserved := make(map[int]bool)
	exclude := ""
	if excludeDataDir != "" {
		exclude = filepath.Clean(excludeDataDir)
	}
	for _, fork := range forks {
		port := fork.Target.Port
		if !validPort(port) {
			continue
		}
		if exclude != "" && filepath.Clean(fork.Target.Data) == exclude {
			continue
		}
		reserved[port] = true
	}
	return reserved
}

func forkPortReservedByManagedFork(dbsu string, port int, dataDir string) bool {
	if !validPort(port) {
		return false
	}
	return reservedManagedForkPorts(dbsu, dataDir)[port]
}

func isPortFree(port int) bool {
	if !validPort(port) {
		return false
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func hasPostmasterPID(dbsu, dataDir string) bool {
	_, err := utils.DBSUCommandOutput(dbsu, []string{"cat", filepath.Join(dataDir, "postmaster.pid")})
	return err == nil
}

func validateForkDataPaths(src, dst string) (string, string, error) {
	srcPath, err := normalizeDataPath(src)
	if err != nil {
		return "", "", err
	}
	dstPath, err := normalizeDataPath(dst)
	if err != nil {
		return "", "", err
	}
	dstLiteral, err := cleanAbsPath(dst)
	if err != nil {
		return "", "", err
	}
	if dstLiteral == "/" || dstLiteral == "/pg" || dstPath == "/" || dstPath == "/pg" {
		return "", "", fmt.Errorf("unsafe destination data directory: %s", dst)
	}
	if srcPath == dstPath {
		return "", "", fmt.Errorf("source and destination data directories must differ: %s", srcPath)
	}
	if pathContains(dstPath, srcPath) {
		return "", "", fmt.Errorf("destination data directory must not be a parent of source data directory: %s", dstPath)
	}
	if pathContains(srcPath, dstPath) {
		return "", "", fmt.Errorf("destination data directory must not be inside source data directory: %s", dstPath)
	}
	return srcPath, dstPath, nil
}

func normalizeDataPath(path string) (string, error) {
	cleaned, err := cleanAbsPath(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return filepath.Clean(resolved), nil
	}

	parent := cleaned
	suffix := []string{}
	for {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		next := filepath.Dir(parent)
		if next == parent {
			return cleaned, nil
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = next
	}
}

func cleanAbsPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("data directory path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func requireCOW(state *State, force bool) error {
	if state != nil && state.CloneMode == CloneModeCOW {
		return nil
	}
	fs := "unknown"
	if state != nil && state.FS != "" {
		fs = state.FS
	}
	if force {
		return nil
	}
	return fmt.Errorf("copy-on-write is not available on source filesystem %q; use --force to allow regular copy fallback", fs)
}

func detectCloneMode(src, dst string) (CloneMode, string) {
	dstParent := existingParent(filepath.Dir(dst))
	srcMount, srcFS := dfMountAndFS(src)
	dstMount, _ := dfMountAndFS(dstParent)
	if srcMount == "" || dstMount == "" || srcMount != dstMount {
		return CloneModeCopy, srcFS
	}
	switch strings.ToLower(srcFS) {
	case "xfs":
		if xfsReflinkEnabled(srcMount) {
			return CloneModeCOW, srcFS
		}
		return CloneModeCopy, srcFS
	case "btrfs", "bcachefs", "ocfs2":
		return CloneModeCOW, srcFS
	default:
		return CloneModeCopy, srcFS
	}
}

func detectCloneModeAs(dbsu, src, dst string) (CloneMode, string) {
	dstParent := existingParentAs(dbsu, filepath.Dir(dst))
	srcMount, srcFS := dfMountAndFSAs(dbsu, src)
	dstMount, _ := dfMountAndFSAs(dbsu, dstParent)
	logrus.Debugf("fork clone mode probe: src=%s dst=%s dstParent=%s srcMount=%s srcFS=%s dstMount=%s", src, dst, dstParent, srcMount, srcFS, dstMount)
	if srcMount == "" || dstMount == "" || srcMount != dstMount {
		return CloneModeCopy, srcFS
	}
	switch strings.ToLower(srcFS) {
	case "xfs":
		if xfsReflinkEnabled(srcMount) {
			return CloneModeCOW, srcFS
		}
		return CloneModeCopy, srcFS
	case "btrfs", "bcachefs", "ocfs2":
		return CloneModeCOW, srcFS
	default:
		return CloneModeCopy, srcFS
	}
}

func xfsReflinkEnabled(mount string) bool {
	for _, bin := range []string{"xfs_info", "/usr/sbin/xfs_info", "/sbin/xfs_info"} {
		out, err := forkXFSInfoOutput(bin, mount)
		if err == nil {
			logrus.Debugf("xfs reflink probe: bin=%s mount=%s reflink=%v", bin, mount, strings.Contains(string(out), "reflink=1"))
			return strings.Contains(string(out), "reflink=1")
		}
		logrus.Debugf("xfs reflink probe failed: bin=%s mount=%s err=%v", bin, mount, err)
	}
	return false
}

func existingParent(path string) string {
	for path != "" && path != "." && path != "/" {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			return path
		}
		path = filepath.Dir(path)
	}
	return "/"
}

func existingParentAs(dbsu, path string) string {
	candidate, err := cleanAbsPath(path)
	if err != nil {
		return "/"
	}
	for candidate != "" && candidate != "." && candidate != "/" {
		if _, err := forkDBSUCommandOutput(dbsu, []string{"test", "-d", candidate}); err == nil {
			return candidate
		}
		candidate = filepath.Dir(candidate)
	}
	return "/"
}

func dfMountAndFS(path string) (string, string) {
	out, err := exec.Command("df", "-T", path).Output()
	if err != nil {
		return "", ""
	}
	return parseDFMountAndFS(string(out))
}

func dfMountAndFSAs(dbsu, path string) (string, string) {
	out, err := forkDBSUCommandOutput(dbsu, []string{"df", "-T", path})
	if err != nil {
		return "", ""
	}
	return parseDFMountAndFS(out)
}

func parseDFMountAndFS(out string) (string, string) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return "", ""
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 7 {
		return "", ""
	}
	return fields[6], fields[1]
}

func instanceResult(opts *Options, state *State, elapsed time.Duration) ResultData {
	inst := opts.Instance
	return ResultData{
		Kind:            KindInstance,
		Source:          inst.SourceData,
		Destination:     inst.DestData,
		SourcePort:      inst.SourcePort,
		DestinationPort: inst.DestPort,
		BackupMode:      string(state.BackupMode),
		CloneMode:       string(state.CloneMode),
		Started:         state.Started,
		ConnectCommand:  utils.ShellQuoteArgs([]string{"psql", "-p", strconv.Itoa(inst.DestPort)}),
		CleanupCommand:  utils.ShellQuoteArgs([]string{"pg_ctl", "-D", inst.DestData, "stop"}) + "; " + utils.ShellQuoteArgs([]string{"rm", "-rf", "--", inst.DestData}),
		Duration:        elapsed.Seconds(),
	}
}

func EscapeSQLString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func quoteArg(value string) string {
	if strings.ContainsAny(value, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return value
}
