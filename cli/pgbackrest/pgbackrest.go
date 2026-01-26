// Package pgbackrest provides pgBackRest backup/restore management for PostgreSQL.
package pgbackrest

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

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
func GetStanza(configPath string) (string, error) {
	stanzas, err := ListStanzaNames(configPath)
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
func ListStanzaNames(configPath string) ([]string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	var stanzas []string
	scanner := bufio.NewScanner(file)
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
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	return stanzas, nil
}

// GetPgPathFromConfig reads pg1-path from the stanza section in config file.
func GetPgPathFromConfig(configPath, stanza string) string {
	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	inStanza := false
	scanner := bufio.NewScanner(file)
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
	if _, err := os.Stat(result.ConfigPath); err != nil {
		return nil, fmt.Errorf("config file not found: %s", result.ConfigPath)
	}
	if result.Stanza == "" {
		stanza, err := GetStanza(result.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("cannot detect stanza: %w (use --stanza to specify)", err)
		}
		result.Stanza = stanza
	}
	return result, nil
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
