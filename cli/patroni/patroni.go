package patroni

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	DefaultLogDir     = "/pg/log/patroni"
	DefaultLogFile    = "patroni.log"
	DefaultConfigPath = "/etc/patroni/patroni.yml"
	InfraConfigPath   = "/infra/conf/patronictl.yml"
)

const conventionalConfigProbe = `if [ -f /etc/patroni/patroni.yml ] && [ -r /etc/patroni/patroni.yml ]; then printf etc; elif [ -f /infra/conf/patronictl.yml ] && [ -r /infra/conf/patronictl.yml ]; then printf infra; else printf fallback; fi`

// CommandOptions are the small Patronictl root-option surface owned by Pig.
type CommandOptions struct {
	ConfigFile string
	DCSURL     string
	Insecure   bool
}

var (
	commandOptions         CommandOptions
	commandOptionsResolved bool
	patroniConfigReadable  = func(path string) bool {
		file, err := os.Open(path)
		if err != nil {
			return false
		}
		info, statErr := file.Stat()
		_ = file.Close()
		return statErr == nil && info.Mode().IsRegular()
	}
)

// SetCommandOptions stores root options without touching the filesystem. This
// keeps pure systemd helpers independent from Patronictl configuration.
func SetCommandOptions(options CommandOptions) {
	commandOptions = options
	commandOptionsResolved = false
}

// ResolveCommandOptions selects exactly one config file for a config-aware
// invocation. Repeated consumers in the same process reuse the decision.
func ResolveCommandOptions(dbsu string) error {
	if commandOptionsResolved {
		return nil
	}
	path, err := ResolveConfigFile(commandOptions.ConfigFile, dbsu)
	if err != nil {
		return &codedPatroniError{code: output.CodePtConfigResolveFailed, err: err}
	}
	commandOptions.ConfigFile = path
	commandOptionsResolved = true
	return nil
}

// ResolveConfigFile implements the normative explicit, environment, /etc,
// /infra, fallback precedence. Conventional readability is checked as DBSU.
func ResolveConfigFile(explicit, dbsu string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		absolute, err := absolutePath(explicit)
		if err != nil {
			return "", fmt.Errorf("resolve explicit Patronictl config path: %w", err)
		}
		logrus.Debugf("patronictl config selected: source=explicit path=%s", absolute)
		return absolute, nil
	}
	environment := os.Getenv("PATRONICTL_CONFIG_FILE")
	if strings.TrimSpace(environment) != "" {
		absolute, err := absolutePath(environment)
		if err != nil {
			return "", fmt.Errorf("resolve PATRONICTL_CONFIG_FILE path: %w", err)
		}
		logrus.Debugf("patronictl config selected: source=environment path=%s", absolute)
		return absolute, nil
	}

	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	if utils.IsDBSU(dbsu) {
		if patroniConfigReadable(DefaultConfigPath) {
			logrus.Debugf("patronictl config selected: source=etc path=%s", DefaultConfigPath)
			return DefaultConfigPath, nil
		}
		logrus.Debugf("patronictl config candidate skipped: path=%s reason=not-readable", DefaultConfigPath)
		if patroniConfigReadable(InfraConfigPath) {
			logrus.Debugf("patronictl config selected: source=infra path=%s", InfraConfigPath)
			return InfraConfigPath, nil
		}
		logrus.Debugf("patronictl config candidate skipped: path=%s reason=not-readable", InfraConfigPath)
		logrus.Debugf("patronictl config fallback: path=%s reason=no-readable-candidate", DefaultConfigPath)
		return DefaultConfigPath, nil
	}

	probe, err := patroniDBSUCommandStdout(dbsu, []string{"sh", "-c", conventionalConfigProbe})
	if err != nil {
		return "", fmt.Errorf("probe conventional Patronictl configs as %s: %w", dbsu, err)
	}
	switch strings.TrimSpace(probe) {
	case "etc":
		logrus.Debugf("patronictl config selected: source=etc path=%s", DefaultConfigPath)
		return DefaultConfigPath, nil
	case "infra":
		logrus.Debugf("patronictl config candidate skipped: path=%s reason=not-readable", DefaultConfigPath)
		logrus.Debugf("patronictl config selected: source=infra path=%s", InfraConfigPath)
		return InfraConfigPath, nil
	case "fallback":
		logrus.Debugf("patronictl config candidate skipped: path=%s reason=not-readable", DefaultConfigPath)
		logrus.Debugf("patronictl config candidate skipped: path=%s reason=not-readable", InfraConfigPath)
		logrus.Debugf("patronictl config fallback: path=%s reason=no-readable-candidate", DefaultConfigPath)
		return DefaultConfigPath, nil
	default:
		return "", fmt.Errorf("unexpected Patronictl config probe result %q", strings.TrimSpace(probe))
	}
}

func absolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

// ConfigFile returns the already-selected config. Callers must resolve command
// options before entering a config-aware workflow.
func ConfigFile() string {
	if commandOptions.ConfigFile != "" {
		return commandOptions.ConfigFile
	}
	return DefaultConfigPath
}

func configFile() string { return ConfigFile() }

func patronictlArgs(binPath string, args ...string) []string {
	result := []string{binPath, "-c", ConfigFile()}
	if commandOptions.DCSURL != "" {
		result = append(result, "--dcs-url", commandOptions.DCSURL)
	}
	if commandOptions.Insecure {
		result = append(result, "--insecure")
	}
	return append(result, args...)
}

// Passthrough launches Patronictl with direct terminal file descriptors. It
// prepends only Pig's selected root options and never inspects native output.
func Passthrough(dbsu string, args []string) error {
	binPath, err := patroniLookPath("patronictl")
	if err != nil {
		return &codedPatroniError{
			code: output.CodePtNotFound,
			err:  fmt.Errorf("patronictl not found in PATH, please install Patroni first"),
		}
	}
	if !filepath.IsAbs(binPath) {
		binPath, err = filepath.Abs(binPath)
		if err != nil {
			return fmt.Errorf("resolve patronictl executable path: %w", err)
		}
	}
	// Resolve the binary in the caller's PATH before a conventional config
	// probe or the native command changes OS identity.
	if err := ResolveCommandOptions(dbsu); err != nil {
		return err
	}
	logrus.Debug("starting patronictl passthrough")
	return patroniPassthroughDBSUCommand(dbsu, patronictlArgs(binPath, args...))
}

// SetRoute identifies the one native edit-config flag for a set pair.
type SetRoute string

const (
	SetRoutePatroni    SetRoute = "set"
	SetRoutePostgreSQL SetRoute = "pg"
)

// SetPair is a validated token. Token remains byte-for-byte as supplied.
type SetPair struct {
	Token string
	Key   string
	Route SetRoute
}

var patroniScalarSetKeys = map[string]struct{}{
	"loop_wait":               {},
	"ttl":                     {},
	"retry_timeout":           {},
	"primary_race_backoff":    {},
	"maximum_lag_on_failover": {},
	"maximum_lag_on_syncnode": {},
	"max_timelines_history":   {},
	"primary_start_timeout":   {},
	"primary_stop_timeout":    {},
	"synchronous_mode":        {},
	"synchronous_mode_strict": {},
	"synchronous_node_count":  {},
	"failsafe_mode":           {},
	"check_timeline":          {},
	"member_slots_ttl":        {},
}

var rejectedStructuralPrefixes = []string{
	"postgresql.",
	"standby_cluster.",
	"slots.",
	"ignore_slots.",
}

// ParseSetPairs validates and classifies complete key=value tokens.
func ParseSetPairs(tokens []string) ([]SetPair, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("at least one key=value pair is required")
	}
	pairs := make([]SetPair, 0, len(tokens))
	for _, token := range tokens {
		equals := strings.IndexByte(token, '=')
		if equals < 0 {
			return nil, fmt.Errorf("expected key=value, got %q", token)
		}
		key := strings.TrimSpace(token[:equals])
		if key == "" {
			return nil, fmt.Errorf("configuration key must not be empty")
		}
		for _, prefix := range rejectedStructuralPrefixes {
			if strings.HasPrefix(key, prefix) {
				return nil, fmt.Errorf("%q is a structural Patroni path; use 'pig pt edit-config --set KEY=VALUE'", key)
			}
		}
		route := SetRoutePostgreSQL
		if _, ok := patroniScalarSetKeys[key]; ok {
			route = SetRoutePatroni
		}
		pairs = append(pairs, SetPair{Token: token, Key: key, Route: route})
	}
	return pairs, nil
}

// BuildSetArgs returns one ordered edit-config operation.
func BuildSetArgs(pairs []SetPair, force bool) []string {
	args := []string{"edit-config"}
	for _, pair := range pairs {
		flag := "--pg"
		if pair.Route == SetRoutePatroni {
			flag = "--set"
		}
		args = append(args, flag, pair.Token)
	}
	if force {
		args = append(args, "--force")
	}
	return args
}

// PostgreSQLSetTokens returns only tokens routed through --pg.
func PostgreSQLSetTokens(pairs []SetPair) []string {
	result := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if pair.Route == SetRoutePostgreSQL {
			result = append(result, pair.Token)
		}
	}
	return result
}

// ApplySet executes exactly one Patronictl edit-config call. Patronictl owns
// the diff, prompt, concurrency check, parsing, and DCS update.
func ApplySet(dbsu string, pairs []SetPair, force bool) error {
	if len(pairs) == 0 {
		return fmt.Errorf("at least one key=value pair is required")
	}
	return Passthrough(dbsu, BuildSetArgs(pairs, force))
}

// GetClusterName reads the configured Patroni scope for local status output.
func GetClusterName(dbsu string) (string, error) {
	path := ConfigFile()
	content, err := patroniReadFile(path)
	if err != nil {
		if dbsu == "" {
			dbsu = utils.GetDBSU("")
		}
		text, dbsuErr := patroniDBSUCommandStdout(dbsu, []string{"cat", path})
		if dbsuErr != nil {
			return "", patroniConfigError(fmt.Errorf("cannot read %s (direct: %v; as %s: %v)", path, err, dbsu, dbsuErr))
		}
		content = []byte(text)
	}
	cluster, err := parseClusterNameFromConfig(string(content))
	if err != nil {
		return "", patroniConfigError(err)
	}
	return cluster, nil
}

// Systemctl operates only the local patroni unit.
func Systemctl(action string) error {
	logrus.Debugf("systemctl %s patroni", action)
	return utils.RunSystemctl(action, "patroni")
}

// Status preserves the combined v1.5.1 local status view.
func Status(dbsu string) error {
	fmt.Println("=== Patroni Service Status ===")
	service := exec.Command("systemctl", "status", "patroni", "--no-pager", "-l")
	service.Stdout = os.Stdout
	service.Stderr = os.Stderr
	if err := service.Run(); err != nil {
		logrus.Debugf("systemctl status patroni: %v (may be expected)", err)
	}

	fmt.Println("\n=== Patroni Processes ===")
	processes := exec.Command("bash", "-c", "ps aux | grep -E '[p]atroni' || echo 'No patroni processes found'")
	processes.Stdout = os.Stdout
	processes.Stderr = os.Stderr
	_ = processes.Run()

	fmt.Println("\n=== Patroni Cluster Status ===")
	binPath, err := patroniLookPath("patronictl")
	if err != nil {
		fmt.Println("patronictl not found, skipping cluster status")
		return nil
	}
	return patroniDBSUCommand(dbsu, patronictlArgs(binPath, "list", "-e", "-t"))
}

// LogDir returns the explicit, configured, or Pigsty-default log directory.
func LogDir(override, dbsu string) string {
	logDir, _ := ResolveLogDir(override, dbsu, false)
	return logDir
}

func ResolveLogDir(override, dbsu string, requireConfig bool) (string, error) {
	if override != "" {
		return override, nil
	}
	content, err := readPatroniLogConfig(dbsu)
	if err != nil {
		if requireConfig {
			return "", err
		}
		return DefaultLogDir, nil
	}
	var cfg struct {
		Log struct {
			Dir string `yaml:"dir"`
		} `yaml:"log"`
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		if requireConfig {
			return "", fmt.Errorf("cannot parse Patroni config %s: %w", ConfigFile(), err)
		}
		return DefaultLogDir, nil
	}
	if dir := strings.TrimSpace(cfg.Log.Dir); dir != "" {
		return dir, nil
	}
	return DefaultLogDir, nil
}

func readPatroniLogConfig(dbsu string) ([]byte, error) {
	path := ConfigFile()
	content, err := patroniReadFile(path)
	if err == nil {
		return content, nil
	}
	if !os.IsPermission(err) {
		return nil, err
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	text, dbsuErr := patroniDBSUCommandStdout(dbsu, []string{"cat", path})
	if dbsuErr != nil {
		return nil, fmt.Errorf("cannot read Patroni config %s: direct: %v; as %s: %w", path, err, dbsu, dbsuErr)
	}
	return []byte(text), nil
}

func resolveCurrentLogFile(logDir string) string {
	return filepath.Join(filepath.Clean(logDir), DefaultLogFile)
}

func LogTail(logDirOverride, dbsu string, lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	logFile := resolveCurrentLogFile(LogDir(logDirOverride, dbsu))
	args := []string{"tail", "-n", fmt.Sprintf("%d", lines), "-f", logFile}
	utils.PrintHint(args)
	return patroniDBSUCommand(dbsu, args)
}

func LogCat(logDirOverride, dbsu string, lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	logFile := resolveCurrentLogFile(LogDir(logDirOverride, dbsu))
	args := []string{"tail", "-n", fmt.Sprintf("%d", lines), logFile}
	utils.PrintHint(args)
	return patroniDBSUCommand(dbsu, args)
}

func LogShowJSONL(logDirOverride, dbsu string, lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	logFile := resolveCurrentLogFile(LogDir(logDirOverride, dbsu))
	out, err := patroniDBSUCommandStdout(dbsu, []string{"tail", "-n", fmt.Sprintf("%d", lines), logFile})
	if err != nil {
		return err
	}
	return utils.PrintLogMessagesJSONL("patroni", out)
}

func LogGrep(logDirOverride, dbsu, pattern string, lines int) error {
	if lines < 0 {
		return fmt.Errorf("lines must be positive")
	}
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	logFile := resolveCurrentLogFile(LogDir(logDirOverride, dbsu))
	args := []string{"grep", "--color=auto", "-i", "--", pattern, logFile}
	if lines > 0 {
		if _, err := patroniDBSUCommandStdout(dbsu, []string{"test", "-r", logFile}); err != nil {
			return fmt.Errorf("cannot read log file %s: %w", logFile, err)
		}
		args = []string{
			"bash", "-o", "pipefail", "-c",
			`tail -n "$1" "$2" | grep --color=auto -i -- "$3"`,
			"bash", fmt.Sprintf("%d", lines), logFile, pattern,
		}
	}
	utils.PrintHint(args)
	err := patroniDBSUCommand(dbsu, args)
	var exitErr *utils.ExitCodeError
	if errors.As(err, &exitErr) && exitErr.Code == 1 {
		return &utils.ExitCodeError{Code: 1, Err: err, Silent: true}
	}
	return err
}
