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
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

type Kind string

const (
	KindInstance Kind = "instance"
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
	Kind Kind
	DbSU string
	Plan bool
	Yes  bool
	// Run is kept for compatibility with the old --run/-r naming. New callers
	// should set Start.
	Run      bool
	Start    bool
	NoStart  bool
	Replace  bool
	Progress bool
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
	Name            string  `json:"name,omitempty" yaml:"name,omitempty"`
	Source          string  `json:"source" yaml:"source"`
	Destination     string  `json:"destination" yaml:"destination"`
	SourcePort      int     `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	DestinationPort int     `json:"destination_port,omitempty" yaml:"destination_port,omitempty"`
	BackupMode      string  `json:"backup_mode,omitempty" yaml:"backup_mode,omitempty"`
	CloneMode       string  `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Started         bool    `json:"started" yaml:"started"`
	Already         bool    `json:"already,omitempty" yaml:"already,omitempty"`
	ConnectCommand  string  `json:"connect_command,omitempty" yaml:"connect_command,omitempty"`
	StartCommand    string  `json:"start_command,omitempty" yaml:"start_command,omitempty"`
	StopCommand     string  `json:"stop_command,omitempty" yaml:"stop_command,omitempty"`
	CleanupCommand  string  `json:"cleanup_command,omitempty" yaml:"cleanup_command,omitempty"`
	PigVersion      string  `json:"pig_version,omitempty" yaml:"pig_version,omitempty"`
	PigRevision     string  `json:"pig_revision,omitempty" yaml:"pig_revision,omitempty"`
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
	Pig       PigBuildInfo   `json:"pig,omitempty" yaml:"pig,omitempty"`
	Orphan    bool           `json:"orphan,omitempty" yaml:"orphan,omitempty"`
}

type PigBuildInfo struct {
	Version  string `json:"version,omitempty" yaml:"version,omitempty"`
	Branch   string `json:"branch,omitempty" yaml:"branch,omitempty"`
	Revision string `json:"revision,omitempty" yaml:"revision,omitempty"`
	BuiltAt  string `json:"built_at,omitempty" yaml:"built_at,omitempty"`
}

type ForkEndpoint struct {
	Data    string `json:"data" yaml:"data"`
	Port    int    `json:"port,omitempty" yaml:"port,omitempty"`
	Started bool   `json:"started,omitempty" yaml:"started,omitempty"`
	PID     int    `json:"pid,omitempty" yaml:"pid,omitempty"`
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
	Progress   bool
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
var forkLstat = os.Lstat
var forkStopPostgres = Stop
var forkProbeSourceDataDir = probeSourceDataDir
var forkConfirmCountdown = confirmForkWithCountdown
var forkXFSInfoOutput = func(bin, mount string) ([]byte, error) {
	return exec.Command(bin, mount).Output()
}

// NormalizeOptions fills in defaults (DBSU, source/destination data dirs and
// ports, managed vs unmanaged destination) and validates the fork options. It
// returns a copy and never mutates the caller's Options.
func NormalizeOptions(opts *Options) (*Options, error) {
	if opts == nil {
		return nil, fmt.Errorf("fork options are required")
	}
	n := *opts
	n.DbSU = utils.GetDBSU(n.DbSU)
	n.Start = (n.Start || n.Run) && !n.NoStart

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
		port, err := firstFreePortAvoiding(15432, reservedManagedForkPorts(opts.DbSU, inst.DestData))
		if err != nil {
			return err
		}
		inst.DestPort = port
	}
	if !validPort(inst.DestPort) {
		return fmt.Errorf("invalid destination port %d (must be 1-65535)", inst.DestPort)
	}
	if inst.Timeout == 0 {
		inst.Timeout = 60
	}
	return nil
}

// Plan returns the dry-run execution plan for a fork without making any changes.
// It probes the source and destination read-only so the plan reflects the copy
// strategy (hot/cold, CoW/regular) execution will actually use.
func Plan(opts *Options) (*output.Plan, error) {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	state, err := prepareNormalized(n)
	if err != nil {
		return nil, err
	}
	return BuildPlan(n, state), nil
}

// Execute runs a fork for the interactive (text) path: it prechecks, prints a
// summary, optionally waits out a countdown when falling back to a regular copy,
// performs the copy/configure/start, and prints a connection hint.
func Execute(opts *Options) error {
	n, err := NormalizeOptions(opts)
	if err != nil {
		return exitForkError(output.CodeForkInvalidArgs, err)
	}
	n.Progress = true
	start := time.Now()
	state, err := prepareNormalized(n)
	if err != nil {
		if fe, ok := err.(*ForkError); ok {
			return &utils.ExitCodeError{Code: output.ExitCode(fe.Code), Err: fe}
		}
		return err
	}
	fmt.Fprint(os.Stderr, forkExecutionSummary(n, state))
	if reason := forkCountdownReason(state); reason != "" {
		if !n.Yes {
			if err := forkConfirmCountdown(reason, "FORK"); err != nil {
				return exitForkError(output.CodeForkInvalidArgs, err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Confirmation wait: skipped by --yes/--force.")
		}
	}
	fmt.Fprintln(os.Stderr, "Starting fork execution...")
	data, err := executePrepared(n, state, start)
	if err != nil {
		if fe, ok := err.(*ForkError); ok {
			return &utils.ExitCodeError{Code: output.ExitCode(fe.Code), Err: fe}
		}
		return err
	}
	fmt.Fprint(os.Stderr, ForkCreateHint(data))
	return nil
}

func forkCountdownReason(state *State) string {
	if state != nil && state.CloneMode == CloneModeCopy {
		return "Copy-on-write is not available; regular copy fallback may consume full data directory space."
	}
	return ""
}

func ForkConnectionHint(data ResultData) string {
	if !data.Started || data.DestinationPort == 0 {
		return ""
	}
	connect := data.ConnectCommand
	if connect == "" {
		connect = forkConnectCommand(data.DestinationPort)
	}
	return fmt.Sprintf("Fork is running on port %d\nConnect: %s\n", data.DestinationPort, connect)
}

func ForkCreateHint(data ResultData) string {
	if data.Destination == "" {
		return ForkConnectionHint(data)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Created: %s (%s)\n", forkResultName(data), data.Destination)
	if data.DestinationPort != 0 {
		fmt.Fprintf(&b, "Port: %d\n", data.DestinationPort)
	}
	if data.Started {
		fmt.Fprintln(&b, "State: running")
		if connect := forkResultConnectCommand(data); connect != "" {
			fmt.Fprintf(&b, "Connect: %s\n", connect)
		}
		if data.StopCommand != "" {
			fmt.Fprintf(&b, "Stop: %s\n", data.StopCommand)
		}
	} else {
		fmt.Fprintln(&b, "State: stopped")
		if data.StartCommand != "" {
			fmt.Fprintf(&b, "Start: %s\n", data.StartCommand)
		}
	}
	if data.CleanupCommand != "" {
		fmt.Fprintf(&b, "Remove: %s\n", data.CleanupCommand)
	}
	return b.String()
}

func ForkActionHint(action string, data ResultData) string {
	switch action {
	case "fork start":
		status := "Started"
		if data.Already {
			status = "Already running"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s: %s (%s)\n", status, forkResultName(data), data.Destination)
		if data.DestinationPort != 0 {
			fmt.Fprintf(&b, "Port: %d\n", data.DestinationPort)
		}
		if connect := forkResultConnectCommand(data); connect != "" {
			fmt.Fprintf(&b, "Connect: %s\n", connect)
		}
		return b.String()
	case "fork stop":
		status := "Stopped"
		if data.Already {
			status = "Already stopped"
		}
		return fmt.Sprintf("%s: %s (%s)\n", status, forkResultName(data), data.Destination)
	case "fork remove":
		return fmt.Sprintf("Removed: %s (%s)\n", forkResultName(data), data.Destination)
	default:
		return ForkConnectionHint(data)
	}
}

func forkResultConnectCommand(data ResultData) string {
	if !data.Started || data.DestinationPort == 0 {
		return ""
	}
	if data.ConnectCommand != "" {
		return data.ConnectCommand
	}
	return forkConnectCommand(data.DestinationPort)
}

func forkResultName(data ResultData) string {
	if data.Name != "" {
		return data.Name
	}
	if data.Destination != "" {
		return strings.TrimPrefix(filepath.Base(data.Destination), "data-")
	}
	return "fork"
}

func forkConnectCommand(port int) string {
	return utils.ShellQuoteArgs([]string{"psql", "-p", strconv.Itoa(port), "-d", "postgres"})
}

func currentPigBuildInfo() PigBuildInfo {
	return PigBuildInfo{
		Version:  config.PigVersion,
		Branch:   config.Branch,
		Revision: config.Revision,
		BuiltAt:  config.BuildDate,
	}
}

// ExecuteResult runs a fork for the structured-output (JSON/YAML) path, returning
// a Result instead of printing. The countdown confirmation is always skipped here.
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

func forkExecutionSummary(opts *Options, state *State) string {
	if opts == nil || opts.Kind != KindInstance {
		return ""
	}
	inst := opts.Instance
	backup := BackupModeUnknown
	clone := CloneModeUnknown
	fs := ""
	if state != nil {
		if state.BackupMode != "" {
			backup = state.BackupMode
		}
		if state.CloneMode != "" {
			clone = state.CloneMode
		}
		fs = state.FS
	}
	var b strings.Builder
	fmt.Fprintln(&b, "PostgreSQL fork summary")
	fmt.Fprintln(&b, "Precheck: OK")
	sourceStatus := "not running"
	if backup == BackupModeHot {
		sourceStatus = "verified"
	}
	targetStatus := "unmanaged"
	if inst.Managed {
		targetStatus = "managed"
	}
	startRequested := opts.Start || opts.Run
	afterCopy := "leave stopped"
	if startRequested {
		afterCopy = "start fork"
	}
	backupLabel := string(backup)
	switch backup {
	case BackupModeHot:
		backupLabel = "hot backup"
	case BackupModeCold:
		backupLabel = "cold copy"
	}
	copyLabel := string(clone)
	switch clone {
	case CloneModeCOW:
		copyLabel = "CoW clone"
	case CloneModeCopy:
		copyLabel = "regular copy"
	}
	if fs != "" {
		copyLabel = fmt.Sprintf("%s (%s)", copyLabel, fs)
	}
	if clone == CloneModeCopy {
		copyLabel += "; may use full data directory space"
	}
	fmt.Fprintf(&b, "Source: %s @ %d (%s)\n", inst.SourceData, inst.SourcePort, sourceStatus)
	fmt.Fprintf(&b, "Target: %s @ %d (%s)\n", inst.DestData, inst.DestPort, targetStatus)
	fmt.Fprintf(&b, "After copy: %s\n", afterCopy)
	fmt.Fprintf(&b, "Backup: %s\n", backupLabel)
	fmt.Fprintf(&b, "Copy: %s\n", copyLabel)
	fmt.Fprintf(&b, "Pig: %s %s/%s %s %s %s\n", config.PigVersion, config.GOOS, config.GOARCH, config.Branch, config.Revision, config.BuildDate)
	fmt.Fprintf(&b, "Command: %s\n", BuildCommand(opts))
	return b.String()
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
	startRequested := opts.Start || opts.Run
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
	if startRequested {
		actions = append(actions, output.Action{Step: step, Description: "Start forked PostgreSQL instance"})
		step++
		actions = append(actions, output.Action{Step: step, Description: "Verify forked instance is reachable"})
	}

	risks := []string{"Destination data directory will be removed when --force is used"}
	if cloneMode == CloneModeCOW {
		risks = append(risks, "Copy-on-write forks share physical blocks until either side writes")
	} else {
		risks = append(risks, "Regular copy fallback may consume full data directory space")
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
			args = append(args, "--src-port", fmt.Sprintf("%d", opts.Instance.SourcePort))
		}
		if opts.Instance.DestData != "" && !opts.Instance.Managed {
			args = append(args, "--dst-data", quoteArg(opts.Instance.DestData))
		}
		if opts.Instance.DestPort != 0 && opts.Instance.DestPort != 15432 {
			args = append(args, "--dst-port", fmt.Sprintf("%d", opts.Instance.DestPort))
		}
		if opts.Start || opts.Run {
			args = append(args, "--start")
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
			Started: opts.Start || opts.Run,
		},
		Copy: ForkCopyInfo{
			Method: "reflink_auto",
			Actual: string(CloneModeUnknown),
		},
		Backup: ForkBackupInfo{
			Mode: string(BackupModeUnknown),
		},
		Commands: ForkCommands{
			Connect: forkConnectCommand(inst.DestPort),
			Stop:    forkStopCommand(inst),
			Remove:  forkRemoveCommand(inst),
		},
		Pig: currentPigBuildInfo(),
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
	dest := filepath.Join(dataDir, "fork.json")
	return forkWriteFileAsDBSU(dest, string(payload), dbsu)
}

func marshalForkInfo(info ForkInfo) ([]byte, error) {
	payload, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	return payload, nil
}

func forkStopCommand(inst InstanceOptions) string {
	if inst.Managed {
		return utils.ShellQuoteArgs([]string{"pig", "pg", "fork", "stop", inst.Name})
	}
	return utils.ShellQuoteArgs([]string{"pig", "pg", "fork", "stop", "--dst-data", inst.DestData})
}

func forkStartCommand(inst InstanceOptions) string {
	if inst.Managed {
		return utils.ShellQuoteArgs([]string{"pig", "pg", "fork", "start", inst.Name})
	}
	return utils.ShellQuoteArgs([]string{"pig", "pg", "fork", "start", "--dst-data", inst.DestData})
}

func forkRemoveCommand(inst InstanceOptions) string {
	if inst.Managed {
		return utils.ShellQuoteArgs([]string{"pig", "pg", "fork", "rm", inst.Name, "--stop"})
	}
	return utils.ShellQuoteArgs([]string{"pig", "pg", "fork", "rm", "--dst-data", inst.DestData, "--stop"})
}

// ScanForksAs enumerates managed forks under root (the data-* directories) as the
// database superuser, reading each fork.json. Directories without valid metadata
// are returned as orphan entries. The result is sorted by fork name.
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

// StartFork starts a previously created fork resolved by name or --dst-data. It is
// idempotent: an already-running fork returns success without restarting.
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
		port, err = firstFreePortAvoiding(15432, reservedManagedForkPorts(dbsu, dataDir))
		if err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkPortInUse, Err: err}
		}
	}
	if !validPort(port) {
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: fmt.Errorf("invalid destination port %d (must be 1-65535)", port)}
	}
	if running, _ := forkCheckPostgresRunning(dbsu, dataDir); running {
		if opts.DestPort != 0 && info.Target.Port != 0 && opts.DestPort != info.Target.Port {
			return ResultData{}, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("fork is already running on port %d; stop it before changing destination port to %d", info.Target.Port, opts.DestPort)}
		}
		if info.Target.Port != 0 {
			port = info.Target.Port
		}
		info.Target.Started = true
		if err := WriteForkInfoAs(dbsu, dataDir, info); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkConfigFailed, Err: err}
		}
		return ResultData{Kind: KindInstance, Name: info.Name, Destination: dataDir, DestinationPort: port, Started: true, Already: true, ConnectCommand: forkConnectCommand(port)}, nil
	}
	if owner, ok := managedForkPortOwner(dbsu, port, dataDir); ok {
		return ResultData{}, &ForkError{Code: output.CodeForkPortInUse, Err: forkPortReservedError(port, owner)}
	}
	if !forkPortFree(port) {
		return ResultData{}, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is in use: %d\nHint: choose another destination port with --dst-port", port)}
	}
	if opts.DestPort != 0 {
		if err := configureInstance(dbsu, dataDir, port); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkConfigFailed, Err: err}
		}
	}
	cfg := &Config{PgData: dataDir, DbSU: dbsu}
	if err := Start(cfg, &StartOptions{Timeout: opts.Timeout, LogFile: filepath.Join(dataDir, "log", "fork.log")}); err != nil {
		return ResultData{}, forkSubprocessError(output.CodeForkStartFailed, err)
	}
	info.Target.Port = port
	info.Target.Started = true
	if err := WriteForkInfoAs(dbsu, dataDir, info); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}
	if err := verifyInstance(dbsu, port); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkVerifyFailed, Err: err}
	}
	return ResultData{Kind: KindInstance, Name: info.Name, Destination: dataDir, DestinationPort: port, Started: true, ConnectCommand: forkConnectCommand(port)}, nil
}

// StopFork stops a running fork resolved by name or --dst-data. It is idempotent:
// an already-stopped fork returns success.
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
	if running, _ := forkCheckPostgresRunning(dbsu, dataDir); !running {
		info.Target.Started = false
		if err := WriteForkInfoAs(dbsu, dataDir, info); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkConfigFailed, Err: err}
		}
		return ResultData{Kind: KindInstance, Name: info.Name, Destination: dataDir, DestinationPort: info.Target.Port, Started: false, Already: true}, nil
	}
	cfg := &Config{PgData: dataDir, DbSU: dbsu}
	if err := forkStopPostgres(cfg, &StopOptions{Mode: opts.StopMode, Timeout: opts.Timeout}); err != nil {
		return ResultData{}, forkSubprocessError(output.CodeForkStopFailed, err)
	}
	info.Target.Started = false
	if err := WriteForkInfoAs(dbsu, dataDir, info); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}
	return ResultData{Kind: KindInstance, Name: info.Name, Destination: dataDir, DestinationPort: info.Target.Port, Started: false}, nil
}

// RemoveFork removes a fork's data directory, resolved by name (managed) or
// --dst-data (unmanaged). A running fork is refused unless StopBefore is set, and
// removal waits out a confirmation countdown unless Yes or Force is set.
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
	if err := validateForkRemovalRoute(opts, dataDir, info); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
	}
	running, pid := forkCheckPostgresRunning(dbsu, dataDir)
	if running {
		if !opts.StopBefore {
			return ResultData{}, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("fork is running (PID: %d): %s\nHint: use --stop to stop and remove it; add -f or -y only to skip the confirmation wait", pid, dataDir)}
		}
	}
	if !opts.Yes && !opts.Force {
		if err := forkConfirmCountdown(fmt.Sprintf("This will remove PostgreSQL fork %s", dataDir), "REMOVE"); err != nil {
			return ResultData{}, &ForkError{Code: output.CodeForkInvalidArgs, Err: err}
		}
	}
	if running {
		cfg := &Config{PgData: dataDir, DbSU: dbsu}
		if err := forkStopPostgres(cfg, &StopOptions{Mode: opts.StopMode, Timeout: opts.Timeout}); err != nil {
			return ResultData{}, forkSubprocessError(output.CodeForkStopFailed, err)
		}
	}
	if opts.Progress {
		fmt.Fprintf(os.Stderr, "%s$ %s%s\n", utils.ColorBlue, forkRemoveCommand(InstanceOptions{Name: info.Name, DestData: dataDir, Managed: info.Managed}), utils.ColorReset)
	}
	if err := removeForkDataDir(dbsu, dataDir); err != nil {
		return ResultData{}, &ForkError{Code: output.CodeForkRemoveFailed, Err: err}
	}
	return ResultData{Kind: KindInstance, Name: info.Name, Destination: dataDir, DestinationPort: info.Target.Port, Started: false}, nil
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
	if !forkDataDirMatches(dbsu, info.Target.Data, dataDir) {
		return ForkInfo{}, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("fork metadata target %s does not match data directory %s", info.Target.Data, dataDir)}
	}
	return info, nil
}

func validateForkRemovalRoute(opts ForkTargetOptions, dataDir string, info ForkInfo) error {
	if opts.DestData != "" {
		if info.Managed {
			name := info.Name
			if name == "" {
				name = filepath.Base(dataDir)
			}
			return fmt.Errorf("managed fork %q must be removed by name, not --dst-data", name)
		}
		return validateUnmanagedForkRemovalPath(dataDir)
	}
	if opts.Name == "" {
		return nil
	}
	if !info.Managed {
		return fmt.Errorf("fork %q is unmanaged; use --dst-data to remove it", opts.Name)
	}
	if info.Name != "" && info.Name != opts.Name {
		return fmt.Errorf("fork metadata name %q does not match requested fork %q", info.Name, opts.Name)
	}
	managedDataDir, err := ManagedForkDataDir(opts.Name)
	if err != nil {
		return err
	}
	if filepath.Clean(dataDir) != managedDataDir {
		return fmt.Errorf("managed fork path invariant violated: %s", dataDir)
	}
	return nil
}

func validateUnmanagedForkRemovalPath(dataDir string) error {
	info, err := forkLstat(dataDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to remove symlink fork data directory: %s", dataDir)
	}
	return nil
}

func forkDataDirMatches(dbsu, metadataDataDir, dataDir string) bool {
	if metadataDataDir == "" || dataDir == "" {
		return false
	}
	if filepath.Clean(metadataDataDir) == filepath.Clean(dataDir) {
		return true
	}
	left, leftErr := resolvePathAsDBSU(dbsu, metadataDataDir)
	right, rightErr := resolvePathAsDBSU(dbsu, dataDir)
	if leftErr != nil || rightErr != nil || left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
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

func prepareNormalized(opts *Options) (*State, error) {
	switch opts.Kind {
	case KindInstance:
		return precheckInstance(opts)
	default:
		return nil, &ForkError{Code: output.CodeForkInvalidArgs, Err: fmt.Errorf("invalid fork kind %q", opts.Kind)}
	}
}

func executeNormalized(opts *Options) (ResultData, error) {
	start := time.Now()
	state, err := prepareNormalized(opts)
	if err != nil {
		return ResultData{}, err
	}
	return executePrepared(opts, state, start)
}

func executePrepared(opts *Options, state *State, start time.Time) (ResultData, error) {
	switch opts.Kind {
	case KindInstance:
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

// forkSubprocessError wraps a subprocess failure in a ForkError carrying the given
// fork code. The fork code (not the subprocess exit code) drives the final process
// exit status, so the inner utils.ExitCodeError is unwrapped to avoid the command
// layer rendering a doubled "command exited with code N: command exited with code M"
// message.
func forkSubprocessError(code int, err error) *ForkError {
	var exitErr *utils.ExitCodeError
	if errors.As(err, &exitErr) && exitErr.Err != nil {
		err = exitErr.Err
	}
	return &ForkError{Code: code, Err: err}
}

func confirmForkWithCountdown(warning, action string) error {
	fmt.Fprintf(os.Stderr, "\n%sWARNING: %s%s\n", utils.ColorYellow, warning, utils.ColorReset)
	fmt.Fprintln(os.Stderr, "Press Ctrl+C within 5 seconds to cancel; use -f or -y to skip this wait.")

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
			fmt.Fprint(os.Stderr, countdownTickMessage(i))
		}
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func countdownTickMessage(seconds int) string {
	return fmt.Sprintf("\rProceeding in %d seconds... ", seconds)
}

func forkProgress(opts *Options, message string) {
	if opts == nil || !opts.Progress || message == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "Step: %s\n", message)
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
		return nil, &ForkError{Code: output.CodeForkSourceNotFound, Err: fmt.Errorf("source data directory is not initialized: %s\nHint: pass a valid source with -D/--src-data, or initialize the source before forking", inst.SourceData)}
	}
	if sourceUsesExternalWALAsDBSU(opts.DbSU, inst.SourceData) {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source data directory uses an external WAL directory via pg_wal: %s\nHint: pig pg fork cannot isolate external WAL directories yet; use pg_basebackup/pgBackRest or a source with WAL inside PGDATA", filepath.Join(inst.SourceData, "pg_wal"))}
	}
	if hasTablespaces, entry, err := sourceHasTablespacesAsDBSU(opts.DbSU, inst.SourceData); err != nil {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: err}
	} else if hasTablespaces {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source data directory uses external tablespaces via pg_tblspc: %s\nHint: pig pg fork cannot isolate external tablespaces yet; use pg_basebackup/pgBackRest with tablespace mapping, or move/drop external tablespaces before forking", entry)}
	}

	if destExists, _ := CheckDataDirAsDBSU(opts.DbSU, inst.DestData); destExists {
		if running, pid := CheckPostgresRunningAsDBSU(opts.DbSU, inst.DestData); running {
			return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("destination fork is running (PID: %d): %s\nHint: stop it with `%s`, or remove it with `%s` before replacing", pid, inst.DestData, forkStopCommand(inst), forkRemoveCommand(inst))}
		}
		if !opts.Replace {
			return nil, &ForkError{Code: output.CodeForkDestExists, Err: fmt.Errorf("destination data directory exists: %s\nHint: use -f/--force to replace a stopped fork, or remove it with `%s`", inst.DestData, forkRemoveCommand(inst))}
		}
	}

	if owner, ok := managedForkPortOwner(opts.DbSU, inst.DestPort, inst.DestData); ok {
		return nil, &ForkError{Code: output.CodeForkPortInUse, Err: forkPortReservedError(inst.DestPort, owner)}
	}
	if !forkPortFree(inst.DestPort) {
		return nil, &ForkError{Code: output.CodeForkPortInUse, Err: fmt.Errorf("destination port is in use: %d\nHint: choose another destination port with --dst-port", inst.DestPort)}
	}

	// Auto-detect the copy mode: take a hot backup if the source is reachable,
	// otherwise fall back to a cold copy. A data directory that still holds a
	// postmaster.pid but is not reachable on the given port is ambiguous (it may be
	// running on another port), so refuse rather than risk copying a live instance.
	running, err := sourcePortMatchesDataDir(opts.DbSU, inst.SourcePort, inst.SourceData)
	if err != nil {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: err}
	}
	if !running && hasPostmasterPID(opts.DbSU, inst.SourceData) {
		return nil, &ForkError{Code: output.CodeForkPrecheckFailed, Err: fmt.Errorf("source instance is not reachable on port %d while %s has postmaster.pid; check --src-port or stop the source before cold copy", inst.SourcePort, inst.SourceData)}
	}

	mode := BackupModeHot
	if !running {
		mode = BackupModeCold
	}
	cloneMode, fs := detectCloneModeAs(opts.DbSU, inst.SourceData, inst.DestData)
	return &State{BackupMode: mode, CloneMode: cloneMode, FS: fs}, nil
}

func executeInstance(opts *Options, state *State) error {
	inst := opts.Instance
	if state.BackupMode == BackupModeCold {
		forkProgress(opts, "copying stopped source data directory")
		if err := copyDataDir(opts.DbSU, inst.SourceData, inst.DestData); err != nil {
			return forkSubprocessError(output.CodeForkCopyFailed, err)
		}
	} else {
		forkProgress(opts, "copying source data directory under PostgreSQL backup")
		if err := hotCopy(opts.DbSU, inst); err != nil {
			return err
		}
	}

	forkProgress(opts, "configuring fork")
	if err := configureInstance(opts.DbSU, inst.DestData, inst.DestPort); err != nil {
		return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}

	forkProgress(opts, "writing fork metadata")
	info := BuildForkInfo(opts, state)
	if err := WriteForkInfoAs(opts.DbSU, inst.DestData, info); err != nil {
		return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
	}

	if opts.Start {
		forkProgress(opts, "starting fork")
		cfg := &Config{PgData: inst.DestData, DbSU: opts.DbSU}
		if err := Start(cfg, forkStartOptions(inst)); err != nil {
			return forkSubprocessError(output.CodeForkStartFailed, err)
		}
		state.Started = true
		forkProgress(opts, "updating fork metadata")
		info = BuildForkInfo(opts, state)
		if err := WriteForkInfoAs(opts.DbSU, inst.DestData, info); err != nil {
			return &ForkError{Code: output.CodeForkConfigFailed, Err: err}
		}
		forkProgress(opts, "verifying fork connection")
		if err := verifyInstance(opts.DbSU, inst.DestPort); err != nil {
			return &ForkError{Code: output.CodeForkVerifyFailed, Err: err}
		}
	}
	return nil
}

func forkStartOptions(inst InstanceOptions) *StartOptions {
	return &StartOptions{
		Timeout: inst.Timeout,
		LogFile: filepath.Join(inst.DestData, "log", "fork.log"),
	}
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
	backupLabel, err := runHotBackupCopy(session, label, copyFn)
	if err != nil {
		var copyErr copyPhaseError
		if errors.As(err, &copyErr) {
			return forkSubprocessError(output.CodeForkCopyFailed, copyErr.Err)
		}
		return &ForkError{Code: output.CodeForkBackupFailed, Err: err}
	}
	if err := writeHotBackupRecoveryFilesAsDBSU(dbsu, inst.SourceData, inst.DestData, backupLabel); err != nil {
		return &ForkError{Code: output.CodeForkBackupFailed, Err: err}
	}
	if err := forkDBSUCommand(dbsu, []string{"test", "-f", filepath.Join(inst.DestData, "PG_VERSION")}); err != nil {
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

func runHotBackupCopy(session backupSession, label string, copyFn func() error) (string, error) {
	version, err := backupServerVersion(session)
	if err != nil {
		return "", err
	}
	names := backupFunctionNames(version)
	if _, err := session.Exec(buildBackupStartSQL(label, names)); err != nil {
		return "", err
	}
	copyErr := copyFn()
	backupLabel, stopErr := session.Exec(buildBackupStopSQL(names))
	if copyErr != nil {
		if stopErr != nil {
			return "", copyPhaseError{Err: fmt.Errorf("copy failed: %w; backup stop also failed: %v", copyErr, stopErr)}
		}
		return "", copyPhaseError{Err: copyErr}
	}
	if stopErr != nil {
		return "", stopErr
	}
	if strings.TrimSpace(backupLabel) == "" {
		return "", fmt.Errorf("%s returned empty backup_label", names.stop)
	}
	return backupLabel, nil
}

func writeBackupLabelAsDBSU(dbsu, dataDir, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("backup_label content is empty")
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return forkWriteFileAsDBSU(filepath.Join(dataDir, "backup_label"), content, dbsu)
}

func writeHotBackupRecoveryFilesAsDBSU(dbsu, sourceData, destData, backupLabel string) error {
	// Refresh the WAL tail written by pg_backup_stop. The initial datadir copy
	// still needs to include the backup start segment; heavy-write sources that
	// may recycle it require a streaming/archive-backed backup workflow.
	if err := copyBackupWALAsDBSU(dbsu, sourceData, destData); err != nil {
		return err
	}
	return writeBackupLabelAsDBSU(dbsu, destData, backupLabel)
}

func copyBackupWALAsDBSU(dbsu, sourceData, destData string) error {
	srcWAL := filepath.Join(sourceData, "pg_wal") + string(os.PathSeparator) + "."
	dstWAL := filepath.Join(destData, "pg_wal") + string(os.PathSeparator)
	return forkDBSUCommand(dbsu, []string{"cp", "-a", "--reflink=auto", srcWAL, dstWAL})
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
		return fmt.Sprintf("SELECT labelfile FROM %s(false, false);\n", names.stop)
	}
	return fmt.Sprintf("SELECT labelfile FROM %s(wait_for_archive => false);\n", names.stop)
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

func sourcePortMatchesDataDir(dbsu string, port int, sourceData string) (bool, error) {
	probedDataDir, err := forkProbeSourceDataDir(dbsu, port)
	if err != nil {
		return false, nil
	}
	if !forkDataDirMatches(dbsu, probedDataDir, sourceData) {
		return true, fmt.Errorf("source port %d data directory %s does not match source data directory %s\nHint: pass matching --src-data and --src-port, or omit both to use the default source", port, probedDataDir, sourceData)
	}
	return true, nil
}

func probeSourceDataDir(dbsu string, port int) (string, error) {
	pg, err := ext.FindPostgres(0)
	if err != nil {
		return "", err
	}
	out, err := forkDBSUCommandOutput(dbsu, []string{pg.Psql(), "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-p", fmt.Sprintf("%d", port), "-d", "postgres", "-c", "SHOW data_directory"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func forkPsqlProbeArgs(psql string, port int) []string {
	return []string{psql, "-X", "-p", fmt.Sprintf("%d", port), "-d", "postgres", "-Atc", "SELECT 1"}
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func firstFreePortAvoiding(start int, reserved map[int]bool) (int, error) {
	if !validPort(start) {
		return 0, fmt.Errorf("invalid destination port search start %d (must be 1-65535)", start)
	}
	end := start + 999
	if end > 65535 {
		end = 65535
	}
	for port := start; port < start+1000 && port <= 65535; port++ {
		if reserved[port] {
			continue
		}
		if forkPortFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free destination port available in range %d-%d", start, end)
}

func reservedManagedForkPorts(dbsu, excludeDataDir string) map[int]bool {
	if _, err := forkLstat("/pg"); err != nil {
		return nil
	}
	forks, err := ScanForksAs(dbsu, "/pg")
	if err != nil {
		logrus.Debugf("scan managed forks for port reservation failed: %v", err)
		return nil
	}
	return reservedForkPortsAs(dbsu, forks, excludeDataDir)
}

func reservedForkPortsAs(dbsu string, forks []ForkInfo, excludeDataDir string) map[int]bool {
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
		if exclude != "" && forkDataDirMatches(dbsu, fork.Target.Data, exclude) {
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
	_, ok := managedForkPortOwner(dbsu, port, dataDir)
	return ok
}

func managedForkPortOwner(dbsu string, port int, excludeDataDir string) (ForkInfo, bool) {
	if !validPort(port) {
		return ForkInfo{}, false
	}
	if _, err := forkLstat("/pg"); err != nil {
		return ForkInfo{}, false
	}
	forks, err := ScanForksAs(dbsu, "/pg")
	if err != nil {
		logrus.Debugf("scan managed forks for port owner failed: %v", err)
		return ForkInfo{}, false
	}
	for _, fork := range forks {
		if fork.Target.Port != port {
			continue
		}
		if excludeDataDir != "" && forkDataDirMatches(dbsu, fork.Target.Data, excludeDataDir) {
			continue
		}
		return fork, true
	}
	return ForkInfo{}, false
}

func forkPortReservedError(port int, owner ForkInfo) error {
	name := owner.Name
	if name == "" {
		name = "unknown"
	}
	data := owner.Target.Data
	if data == "" {
		data = "unknown data directory"
	}
	return fmt.Errorf("destination port %d is reserved by managed fork %s (%s)\nHint: run `pig pg fork list`, or choose another destination port with --dst-port", port, name, data)
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

func sourceUsesExternalWALAsDBSU(dbsu, dataDir string) bool {
	err := forkDBSUCommand(dbsu, []string{"test", "-L", filepath.Join(dataDir, "pg_wal")})
	return err == nil
}

func sourceHasTablespacesAsDBSU(dbsu, dataDir string) (bool, string, error) {
	tblspc := filepath.Join(dataDir, "pg_tblspc")
	if err := forkDBSUCommand(dbsu, []string{"test", "-d", tblspc}); err != nil {
		return false, "", nil
	}
	out, err := forkDBSUCommandOutput(dbsu, []string{"find", tblspc, "-mindepth", "1", "-maxdepth", "1", "-print", "-quit"})
	if err != nil {
		return false, "", err
	}
	entry := strings.TrimSpace(out)
	return entry != "", entry, nil
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
		Name:            inst.Name,
		Source:          inst.SourceData,
		Destination:     inst.DestData,
		SourcePort:      inst.SourcePort,
		DestinationPort: inst.DestPort,
		BackupMode:      string(state.BackupMode),
		CloneMode:       string(state.CloneMode),
		Started:         state.Started,
		ConnectCommand:  forkConnectCommand(inst.DestPort),
		StartCommand:    forkStartCommand(inst),
		StopCommand:     forkStopCommand(inst),
		CleanupCommand:  forkRemoveCommand(inst),
		PigVersion:      config.PigVersion,
		PigRevision:     config.Revision,
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
