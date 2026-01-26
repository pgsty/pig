// Package pgbackrest provides pgBackRest backup/restore management for PostgreSQL.
package pgbackrest

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"pig/internal/config"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// Default paths and settings (Pigsty conventions)
const (
	DefaultConfigPath = "/etc/pgbackrest/pgbackrest.conf"
	DefaultLogDir     = "/pg/log/pgbackrest"
)

// Config holds pgBackRest configuration.
type Config struct {
	ConfigPath string // pgBackRest config file path
	Stanza     string // Stanza name
	Repo       string // Repository number (1, 2, etc.) for multi-repo setups
	DbSU       string // Database superuser (default: postgres)
}

// DefaultConfig returns a new Config with default values.
func DefaultConfig() *Config {
	return &Config{
		ConfigPath: DefaultConfigPath,
		DbSU:       utils.GetDBSU(""),
	}
}

// Pre-compiled regex patterns
var (
	// sectionRegex matches INI section headers like [pg-meta]
	sectionRegex = regexp.MustCompile(`^\[([^\]:]+)\]`)
	// repoConfigRegex matches repo config lines like "repo1-type = posix"
	repoConfigRegex = regexp.MustCompile(`^(repo\d+)-(.+?)\s*=\s*(.+)$`)
	// pgPathRegex matches pg path config like "pg1-path = /pg/data"
	pgPathRegex = regexp.MustCompile(`^pg\d*-path\s*=\s*(.+)$`)
	// pgPortRegex matches pg port config like "pg1-port = 5432"
	pgPortRegex = regexp.MustCompile(`^pg\d*-port\s*=\s*(.+)$`)
)

// GetStanza extracts the first non-global stanza name from config file.
// It looks for section headers like [pg-meta] and skips [global*] sections.
func GetStanza(configPath, dbsu string) (string, error) {
	stanzas, err := ListStanzaNames(configPath, dbsu)
	if err != nil {
		return "", err
	}
	if len(stanzas) == 0 {
		return "", fmt.Errorf("no stanza found in config file")
	}
	if len(stanzas) > 1 {
		logrus.Warnf("multiple stanzas found: %v, using first: %s", stanzas, stanzas[0])
		logrus.Warnf("use --stanza to specify a different stanza")
	}
	return stanzas[0], nil
}

// ListStanzaNames returns all non-global stanza names from config file.
// Uses DBSU privilege escalation if needed.
func ListStanzaNames(configPath, dbsu string) ([]string, error) {
	content, err := readConfigFile(configPath, dbsu)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}

	var stanzas []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := sectionRegex.FindStringSubmatch(line); matches != nil {
			section := matches[1]
			if !strings.HasPrefix(section, "global") {
				stanzas = append(stanzas, section)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}
	return stanzas, nil
}

// GetPgPathFromConfig reads pg1-path from the stanza section in config file.
// Uses DBSU privilege escalation if needed.
func GetPgPathFromConfig(configPath, stanza, dbsu string) string {
	content, err := readConfigFile(configPath, dbsu)
	if err != nil {
		return ""
	}

	inStanza := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if matches := sectionRegex.FindStringSubmatch(line); matches != nil {
			inStanza = matches[1] == stanza
			continue
		}
		if inStanza {
			if matches := pgPathRegex.FindStringSubmatch(line); matches != nil {
				return strings.TrimSpace(matches[1])
			}
		}
	}
	return ""
}

// readConfigFile reads the config file content, using DBSU privilege escalation if needed.
// Execution strategy:
//   - If current user is DBSU: read directly
//   - If current user is root: use "su - <dbsu> -c cat"
//   - Otherwise: try direct read first, then fallback to "sudo -inu <dbsu> cat"
func readConfigFile(configPath, dbsu string) (string, error) {
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}

	// If current user is DBSU, read directly
	if utils.IsDBSU(dbsu) {
		return readFileDirect(configPath)
	}

	// If current user is root, use su
	if config.CurrentUser == "root" {
		return readFileAsSU(configPath, dbsu)
	}

	// Otherwise: try direct first, then sudo fallback
	content, err := readFileDirect(configPath)
	if err == nil {
		return content, nil
	}

	// Check if it's a permission error
	if os.IsPermission(err) {
		logrus.Debugf("permission denied reading %s, trying as %s", configPath, dbsu)
		return readFileAsSudo(configPath, dbsu)
	}

	return "", err
}

// readFileDirect reads file directly
func readFileDirect(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// readFileAsSU reads file using su - dbsu -c "cat file"
func readFileAsSU(path, dbsu string) (string, error) {
	cmd := exec.Command("su", "-", dbsu, "-c", fmt.Sprintf("cat '%s'", path))
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("su failed: %w: %s", err, stderr.String())
	}
	return out.String(), nil
}

// readFileAsSudo reads file using sudo -inu dbsu cat file
func readFileAsSudo(path, dbsu string) (string, error) {
	cmd := exec.Command("sudo", "-inu", dbsu, "cat", path)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sudo failed: %w: %s", err, stderr.String())
	}
	return out.String(), nil
}

// GetEffectiveConfig returns merged config with defaults and auto-detection.
// Priority: explicit flags > auto-detected values > defaults
func GetEffectiveConfig(cfg *Config) (*Config, error) {
	result := &Config{
		ConfigPath: cfg.ConfigPath,
		Stanza:     cfg.Stanza,
		Repo:       cfg.Repo,
		DbSU:       cfg.DbSU,
	}

	if result.ConfigPath == "" {
		result.ConfigPath = DefaultConfigPath
	}
	if result.DbSU == "" {
		result.DbSU = utils.GetDBSU("")
	}

	// Check config file exists (use DBSU if needed for permission)
	if err := checkConfigExists(result.ConfigPath, result.DbSU); err != nil {
		return nil, err
	}

	if result.Stanza == "" {
		stanza, err := GetStanza(result.ConfigPath, result.DbSU)
		if err != nil {
			return nil, fmt.Errorf("cannot detect stanza: %w (use --stanza to specify)", err)
		}
		result.Stanza = stanza
	}
	return result, nil
}

// checkConfigExists checks if config file exists, using DBSU privilege if needed
func checkConfigExists(configPath, dbsu string) error {
	// Try direct stat first
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsPermission(err) {
		// Not a permission error - file doesn't exist
		return fmt.Errorf("config file not found: %s", configPath)
	}

	// Permission denied - try reading with DBSU to verify existence
	_, err := readConfigFile(configPath, dbsu)
	if err != nil {
		return fmt.Errorf("config file not accessible: %s", configPath)
	}
	return nil
}

// buildPgBackRestArgs builds the argument list for pgbackrest command.
func buildPgBackRestArgs(cfg *Config, command string, extraArgs []string) ([]string, error) {
	bin, err := exec.LookPath("pgbackrest")
	if err != nil {
		return nil, fmt.Errorf("pgbackrest not found (install with: pig ext add pgbackrest)")
	}

	args := []string{bin}
	if cfg.ConfigPath != "" && cfg.ConfigPath != DefaultConfigPath {
		args = append(args, "--config="+cfg.ConfigPath)
	}
	if cfg.Stanza != "" {
		args = append(args, "--stanza="+cfg.Stanza)
	}
	if cfg.Repo != "" {
		args = append(args, "--repo="+cfg.Repo)
	}
	args = append(args, extraArgs...)
	args = append(args, command)
	return args, nil
}

// RunPgBackRest executes a pgbackrest command as DBSU.
// If hint is true, prints the command before execution.
func RunPgBackRest(cfg *Config, command string, extraArgs []string, hint bool) error {
	args, err := buildPgBackRestArgs(cfg, command, extraArgs)
	if err != nil {
		return err
	}
	if hint {
		utils.PrintHint(args)
	}
	return utils.DBSUCommand(cfg.DbSU, args)
}

// RunPgBackRestOutput executes pgbackrest and captures output.
func RunPgBackRestOutput(cfg *Config, command string, extraArgs []string) (string, error) {
	args, err := buildPgBackRestArgs(cfg, command, extraArgs)
	if err != nil {
		return "", err
	}
	dbsu := cfg.DbSU
	if dbsu == "" {
		dbsu = utils.GetDBSU("")
	}
	return utils.DBSUCommandOutput(dbsu, args)
}

// CheckPgBackRestExists verifies pgbackrest is installed.
func CheckPgBackRestExists() error {
	_, err := exec.LookPath("pgbackrest")
	if err != nil {
		return fmt.Errorf("pgbackrest not found (install with: pig ext add pgbackrest)")
	}
	return nil
}
