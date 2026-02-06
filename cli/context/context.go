/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pig context command - collects environment context snapshot for AI agents.
*/
package context

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"pig/cli/ext"
	"pig/cli/patroni"
	"pig/cli/pgbackrest"
	"pig/cli/postgres"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// ContextResultData is the top-level context snapshot structure.
// It contains information about the host, PostgreSQL, Patroni, pgBackRest, and extensions.
type ContextResultData struct {
	Host       *HostInfo          `json:"host,omitempty" yaml:"host,omitempty"`
	Postgres   *PostgresContext   `json:"postgres,omitempty" yaml:"postgres,omitempty"`
	Patroni    *PatroniContext    `json:"patroni,omitempty" yaml:"patroni,omitempty"`
	PgBackRest *PgBackRestContext `json:"pgbackrest,omitempty" yaml:"pgbackrest,omitempty"`
	Extensions *ExtensionsContext `json:"extensions,omitempty" yaml:"extensions,omitempty"`
}

// HostInfo contains basic host information.
type HostInfo struct {
	Hostname string `json:"hostname" yaml:"hostname"`
	OS       string `json:"os" yaml:"os"`                             // linux, darwin
	Distro   string `json:"distro,omitempty" yaml:"distro,omitempty"` // el9, d12, u24
	Arch     string `json:"arch" yaml:"arch"`                         // amd64, arm64
	Kernel   string `json:"kernel,omitempty" yaml:"kernel,omitempty"`
}

// PostgresContext contains PostgreSQL instance information.
type PostgresContext struct {
	Available      bool   `json:"available" yaml:"available"`
	Running        bool   `json:"running,omitempty" yaml:"running,omitempty"`
	Version        int    `json:"version,omitempty" yaml:"version,omitempty"`
	VersionString  string `json:"version_string,omitempty" yaml:"version_string,omitempty"`
	VersionNum     int    `json:"version_num,omitempty" yaml:"version_num,omitempty"` // e.g., 170000 for PG17
	DataDir        string `json:"data_dir,omitempty" yaml:"data_dir,omitempty"`
	Port           int    `json:"port,omitempty" yaml:"port,omitempty"`
	PID            int    `json:"pid,omitempty" yaml:"pid,omitempty"`
	Role           string `json:"role,omitempty" yaml:"role,omitempty"` // primary, standby, unknown
	UptimeSeconds  int64  `json:"uptime_seconds,omitempty" yaml:"uptime_seconds,omitempty"`
	Connections    int    `json:"connections,omitempty" yaml:"connections,omitempty"`         // current connection count
	MaxConnections int    `json:"max_connections,omitempty" yaml:"max_connections,omitempty"` // max_connections setting
}

// PatroniContext contains Patroni cluster information.
type PatroniContext struct {
	Available bool   `json:"available" yaml:"available"`
	Running   bool   `json:"running,omitempty" yaml:"running,omitempty"`
	Cluster   string `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Role      string `json:"role,omitempty" yaml:"role,omitempty"`         // leader, replica, standby_leader
	State     string `json:"state,omitempty" yaml:"state,omitempty"`       // running, starting, stopped
	Timeline  int    `json:"timeline,omitempty" yaml:"timeline,omitempty"` // current timeline
	Lag       string `json:"lag,omitempty" yaml:"lag,omitempty"`           // replication lag (e.g., "0 MB", "1.5 MB")
}

// PgBackRestContext contains pgBackRest backup information.
type PgBackRestContext struct {
	Available      bool   `json:"available" yaml:"available"`
	Configured     bool   `json:"configured,omitempty" yaml:"configured,omitempty"`
	Stanza         string `json:"stanza,omitempty" yaml:"stanza,omitempty"`
	LastBackup     string `json:"last_backup,omitempty" yaml:"last_backup,omitempty"`           // backup label
	LastBackupTime int64  `json:"last_backup_time,omitempty" yaml:"last_backup_time,omitempty"` // Unix timestamp of last backup
	BackupCount    int    `json:"backup_count,omitempty" yaml:"backup_count,omitempty"`
}

// ExtensionsContext contains installed extensions information.
type ExtensionsContext struct {
	Available      bool     `json:"available" yaml:"available"`
	InstalledCount int      `json:"installed_count" yaml:"installed_count"`
	Extensions     []string `json:"extensions,omitempty" yaml:"extensions,omitempty"` // list of extension names
}

const (
	ModuleHost       = "host"
	ModulePostgres   = "postgres"
	ModulePatroni    = "patroni"
	ModulePgBackRest = "pgbackrest"
	ModuleExtensions = "extensions"
)

var ValidModules = []string{
	ModuleHost,
	ModulePostgres,
	ModulePatroni,
	ModulePgBackRest,
	ModuleExtensions,
}

const (
	colorGray = "\033[90m"
)

// IsValidModule checks if a module name is valid.
func IsValidModule(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, m := range ValidModules {
		if m == name {
			return true
		}
	}
	return false
}

// ParseModuleFilter parses the module filter flag value into a list of modules.
// It accepts comma-separated module names and preserves negation prefix (!).
// Returns nil for empty input (meaning no filter).
func ParseModuleFilter(filter string) []string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	parts := strings.Split(filter, ",")
	modules := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		modules = append(modules, part)
	}
	if len(modules) == 0 {
		return nil
	}
	return modules
}

type moduleFilter struct {
	include    map[string]struct{}
	exclude    map[string]struct{}
	hasInclude bool
}

func buildModuleFilter(modules []string) (*moduleFilter, error) {
	if len(modules) == 0 {
		return nil, nil
	}
	filter := &moduleFilter{
		include: map[string]struct{}{},
		exclude: map[string]struct{}{},
	}
	for _, raw := range modules {
		name := strings.TrimSpace(strings.ToLower(raw))
		if name == "" {
			continue
		}
		negated := false
		if strings.HasPrefix(name, "!") {
			negated = true
			name = strings.TrimSpace(strings.TrimPrefix(name, "!"))
		}
		if name == "" {
			continue
		}
		if !IsValidModule(name) {
			return nil, fmt.Errorf("invalid module '%s', valid modules: %s", raw, strings.Join(ValidModules, ", "))
		}
		if negated {
			filter.exclude[name] = struct{}{}
		} else {
			filter.include[name] = struct{}{}
			filter.hasInclude = true
		}
	}
	if len(filter.include) == 0 && len(filter.exclude) == 0 {
		return nil, nil
	}
	return filter, nil
}

func (f *moduleFilter) includeModule(module string) bool {
	if f == nil {
		return true
	}
	module = strings.ToLower(strings.TrimSpace(module))
	if module == "" {
		return false
	}
	if _, excluded := f.exclude[module]; excluded {
		return false
	}
	if !f.hasInclude {
		return true
	}
	if module == ModuleHost {
		return true
	}
	_, included := f.include[module]
	return included
}

// Text returns a human-friendly text representation of the context snapshot.
// Returns an empty string if the receiver is nil.
func (c *ContextResultData) Text() string {
	if c == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("=== PIG CONTEXT ===\n")

	// Host info
	if c.Host != nil {
		sb.WriteString(c.Host.text())
	}

	// PostgreSQL info
	if c.Postgres != nil {
		sb.WriteString(c.Postgres.text())
	}

	// Patroni info
	if c.Patroni != nil {
		sb.WriteString(c.Patroni.text())
	}

	// pgBackRest info
	if c.PgBackRest != nil {
		sb.WriteString(c.PgBackRest.text())
	}

	// Extensions info
	if c.Extensions != nil {
		sb.WriteString(c.Extensions.text())
	}

	return sb.String()
}

// text returns formatted text for HostInfo
func (h *HostInfo) text() string {
	if h == nil {
		return ""
	}
	var sb strings.Builder
	distroInfo := ""
	if h.Distro != "" {
		distroInfo = fmt.Sprintf(" (%s/%s)", h.Distro, h.Arch)
	} else {
		distroInfo = fmt.Sprintf(" (%s)", h.Arch)
	}
	sb.WriteString(fmt.Sprintf("Host: %s%s\n", h.Hostname, distroInfo))
	if h.Kernel != "" {
		sb.WriteString(fmt.Sprintf("  OS: %s  Kernel: %s\n", h.OS, h.Kernel))
	}
	return sb.String()
}

// text returns formatted text for PostgresContext
func (p *PostgresContext) text() string {
	if p == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n")
	if !p.Available {
		sb.WriteString(fmt.Sprintf("PostgreSQL: %s Not Available\n", colorizeStatus("○", colorGray)))
		return sb.String()
	}

	statusIcon := "○"
	statusText := "Stopped"
	statusColor := utils.ColorRed
	if p.Running {
		statusIcon = "●"
		statusText = "Running"
		statusColor = utils.ColorGreen
	}
	sb.WriteString(fmt.Sprintf("PostgreSQL: %s %s\n", colorizeStatus(statusIcon, statusColor), statusText))

	if p.Running {
		// Version info with VersionNum
		versionStr := ""
		if p.VersionString != "" {
			versionStr = p.VersionString
		} else if p.Version > 0 {
			versionStr = fmt.Sprintf("%d", p.Version)
		}
		if p.VersionNum > 0 {
			versionStr = fmt.Sprintf("%s (%d)", versionStr, p.VersionNum)
		}
		if versionStr != "" || p.Port > 0 || p.PID > 0 {
			sb.WriteString(fmt.Sprintf("  Version: %s  Port: %d  PID: %d\n", versionStr, p.Port, p.PID))
		}
		if p.DataDir != "" {
			sb.WriteString(fmt.Sprintf("  Data Dir: %s\n", p.DataDir))
		}
		roleInfo := ""
		if p.Role != "" {
			roleInfo = fmt.Sprintf("Role: %s", p.Role)
		}
		uptimeInfo := ""
		if p.UptimeSeconds > 0 {
			uptimeInfo = fmt.Sprintf("Uptime: %s", formatDuration(p.UptimeSeconds))
		}
		if roleInfo != "" || uptimeInfo != "" {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", roleInfo, uptimeInfo))
		}
		// Connection info
		if p.MaxConnections > 0 {
			sb.WriteString(fmt.Sprintf("  Connections: %d/%d\n", p.Connections, p.MaxConnections))
		}
	} else if p.DataDir != "" {
		sb.WriteString(fmt.Sprintf("  Data Dir: %s\n", p.DataDir))
	}
	return sb.String()
}

// text returns formatted text for PatroniContext
func (p *PatroniContext) text() string {
	if p == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n")
	if !p.Available {
		sb.WriteString(fmt.Sprintf("Patroni: %s Not Available\n", colorizeStatus("○", colorGray)))
		return sb.String()
	}

	statusIcon := "○"
	statusText := "Stopped"
	statusColor := utils.ColorRed
	if p.Running {
		statusIcon = "●"
		statusText = "Running"
		statusColor = utils.ColorGreen
	}
	sb.WriteString(fmt.Sprintf("Patroni: %s %s\n", colorizeStatus(statusIcon, statusColor), statusText))

	if p.Cluster != "" || p.Role != "" {
		sb.WriteString(fmt.Sprintf("  Cluster: %s  Role: %s\n", p.Cluster, p.Role))
	}
	if p.State != "" && p.State != "running" {
		sb.WriteString(fmt.Sprintf("  State: %s\n", p.State))
	}
	// Display timeline and lag for running instances
	if p.Running && (p.Timeline > 0 || p.Lag != "") {
		tlStr := "-"
		if p.Timeline > 0 {
			tlStr = fmt.Sprintf("%d", p.Timeline)
		}
		lagStr := "-"
		if p.Lag != "" {
			lagStr = p.Lag
		}
		sb.WriteString(fmt.Sprintf("  Timeline: %s  Lag: %s\n", tlStr, lagStr))
	}
	return sb.String()
}

// text returns formatted text for PgBackRestContext
func (p *PgBackRestContext) text() string {
	if p == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n")
	if !p.Available {
		sb.WriteString(fmt.Sprintf("pgBackRest: %s Not Available\n", colorizeStatus("○", colorGray)))
		return sb.String()
	}

	statusIcon := "○"
	statusText := "Not Configured"
	statusColor := utils.ColorYellow
	if p.Configured {
		statusIcon = "●"
		statusText = "Configured"
		statusColor = utils.ColorGreen
	}
	sb.WriteString(fmt.Sprintf("pgBackRest: %s %s\n", colorizeStatus(statusIcon, statusColor), statusText))

	if p.Stanza != "" {
		sb.WriteString(fmt.Sprintf("  Stanza: %s  Backups: %d\n", p.Stanza, p.BackupCount))
	}
	if p.LastBackup != "" {
		lastInfo := p.LastBackup
		if p.LastBackupTime > 0 {
			// Format time as human-friendly relative time
			lastTime := time.Unix(p.LastBackupTime, 0)
			ago := formatTimeAgo(lastTime)
			lastInfo = fmt.Sprintf("%s (%s)", p.LastBackup, ago)
		}
		sb.WriteString(fmt.Sprintf("  Last: %s\n", lastInfo))
	}
	return sb.String()
}

// formatTimeAgo formats a time as a human-friendly relative time string.
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)
	if duration < 0 {
		return "in future"
	}

	hours := int(duration.Hours())
	if hours < 1 {
		minutes := int(duration.Minutes())
		if minutes < 1 {
			return "just now"
		}
		return fmt.Sprintf("%dm ago", minutes)
	}
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}

	days := hours / 24
	if days < 7 {
		return fmt.Sprintf("%dd ago", days)
	}

	weeks := days / 7
	if weeks < 4 {
		return fmt.Sprintf("%dw ago", weeks)
	}

	months := days / 30
	return fmt.Sprintf("%dmo ago", months)
}

// text returns formatted text for ExtensionsContext
func (e *ExtensionsContext) text() string {
	if e == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n")
	if !e.Available {
		sb.WriteString("Extensions: ○ Not Available\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Extensions: %d installed\n", e.InstalledCount))
	if len(e.Extensions) > 0 {
		// Show first 10 extensions
		displayExts := e.Extensions
		if len(displayExts) > 10 {
			displayExts = displayExts[:10]
		}
		sb.WriteString(fmt.Sprintf("  %s", strings.Join(displayExts, ", ")))
		if len(e.Extensions) > 10 {
			sb.WriteString(fmt.Sprintf(", ... (+%d more)", len(e.Extensions)-10))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatDuration formats seconds into a human-readable duration string
func formatDuration(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func colorizeStatus(text, color string) string {
	if !isColorEnabled() || color == "" {
		return text
	}
	return color + text + utils.ColorReset
}

// isColorEnabled checks if terminal color output should be enabled.
// Returns false if NO_COLOR is set, TERM=dumb, or stdout is not a TTY.
func isColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ContextResultWithModules creates a structured result for pig context command with module filtering.
// Returns nil-safe Result on all paths.
func ContextResultWithModules(modules []string) *output.Result {
	filter, err := buildModuleFilter(modules)
	if err != nil {
		return output.Fail(output.CodeCtxInvalidModule, err.Error())
	}
	data := &ContextResultData{}

	// Collect host info (always available)
	if filter.includeModule(ModuleHost) {
		data.Host = collectHostInfo()
	}

	// Collect PostgreSQL context (graceful degradation)
	if filter.includeModule(ModulePostgres) {
		data.Postgres = collectPostgresContext()
	}

	// Collect Patroni context (graceful degradation)
	if filter.includeModule(ModulePatroni) {
		data.Patroni = collectPatroniContext()
	}

	// Collect pgBackRest context (graceful degradation)
	if filter.includeModule(ModulePgBackRest) {
		data.PgBackRest = collectPgBackRestContext()
	}

	// Collect extensions context (graceful degradation)
	if filter.includeModule(ModuleExtensions) {
		data.Extensions = collectExtensionsContext()
	}

	return output.OK("Environment context collected", data)
}

// collectHostInfo collects basic host information
func collectHostInfo() *HostInfo {
	hostname, _ := os.Hostname()

	host := &HostInfo{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     config.OSArch,
		Distro:   config.OSCode,
	}

	// Get kernel version
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if out, err := exec.Command("uname", "-r").Output(); err == nil {
			host.Kernel = strings.TrimSpace(string(out))
		}
	}

	return host
}

// collectPostgresContext collects PostgreSQL instance information
func collectPostgresContext() *PostgresContext {
	ctx := &PostgresContext{Available: false}

	// Use the postgres.StatusResult to get status information
	cfg := &postgres.Config{}
	result := postgres.StatusResult(cfg)
	if result == nil {
		logrus.Debug("postgres.StatusResult returned nil")
		return ctx
	}

	// Check if we got data back
	if result.Data == nil {
		logrus.Debug("postgres.StatusResult data is nil")
		return ctx
	}

	statusData, ok := result.Data.(*postgres.PgStatusResultData)
	if !ok {
		logrus.Debugf("unexpected data type from postgres.StatusResult: %T", result.Data)
		return ctx
	}

	// If data directory is not found, treat PostgreSQL as not installed
	if result.Code == output.CodePgStatusDataDirNotFound {
		logrus.Debugf("postgres data directory not found: %s", statusData.DataDir)
		return ctx
	}

	ctx.Available = true
	ctx.Running = statusData.Running
	ctx.Version = statusData.Version
	ctx.DataDir = statusData.DataDir
	ctx.Port = statusData.Port
	ctx.PID = statusData.PID
	ctx.UptimeSeconds = statusData.UptimeSeconds

	// Try to read full version string from PG_VERSION for accurate version_num (incl. 9.x)
	if ctx.DataDir != "" {
		if pgVersion := readPgVersionString(cfg, ctx.DataDir); pgVersion != "" {
			ctx.VersionString = "PG" + pgVersion
			if major, _ := parseVersionParts(pgVersion); major > 0 && ctx.Version == 0 {
				ctx.Version = major
			}
			if versionNum := calculateVersionNumFromString(pgVersion); versionNum > 0 {
				ctx.VersionNum = versionNum
			}
		}
	}

	// Fallback to major-only version info
	if ctx.VersionString == "" && ctx.Version > 0 {
		ctx.VersionString = fmt.Sprintf("PG%d", ctx.Version)
	}
	if ctx.VersionNum == 0 && ctx.Version > 0 {
		ctx.VersionNum = calculateVersionNum(ctx.Version)
	}

	// Detect role (primary/standby) - check recovery mode
	if ctx.DataDir != "" {
		ctx.Role = detectPostgresRole(cfg, ctx.DataDir)
	}

	// Collect connection info (only when running)
	if ctx.Running && ctx.Port > 0 {
		dbsu := postgres.GetDbSU(cfg)
		conn, maxConn, err := collectConnectionInfo(ctx.Port, dbsu)
		if err == nil {
			ctx.Connections = conn
			ctx.MaxConnections = maxConn
		} else {
			logrus.Debugf("failed to collect connection info: %v", err)
		}
	}

	return ctx
}

// detectPostgresRole determines if PostgreSQL is primary or standby.
// It checks for recovery/standby signal files with appropriate DBSU privileges.
func detectPostgresRole(cfg *postgres.Config, dataDir string) string {
	dbsu := postgres.GetDbSU(cfg)

	// Check for standby.signal, recovery.signal (PG12+), or recovery.conf (PG11-)
	standbySignal := dataDir + "/standby.signal"
	recoverySignal := dataDir + "/recovery.signal"
	recoveryConf := dataDir + "/recovery.conf"

	inconclusive := false
	for _, signal := range []string{standbySignal, recoverySignal, recoveryConf} {
		exists, err := checkFileExistsAsDBSU(dbsu, signal)
		if err != nil {
			inconclusive = true
			continue
		}
		if exists {
			return "standby"
		}
	}

	if inconclusive {
		return "unknown"
	}
	return "primary"
}

// detectPostgresRoleFromDir determines role by directly checking files in the data directory.
// This is a simpler version that doesn't require DBSU privileges (for use when we have direct access).
// Returns "primary" if no signal files found, "standby" if any recovery signal found,
// "unknown" if directory doesn't exist or is inaccessible.
func detectPostgresRoleFromDir(dataDir string) string {
	// Check if directory exists
	if _, err := os.Stat(dataDir); err != nil {
		return "unknown"
	}

	// Check for standby.signal (PG12+ standby)
	standbySignal := dataDir + "/standby.signal"
	if _, err := os.Stat(standbySignal); err == nil {
		return "standby"
	} else if !os.IsNotExist(err) {
		return "unknown"
	}

	// Check for recovery.signal (PG12+ recovery mode)
	recoverySignal := dataDir + "/recovery.signal"
	if _, err := os.Stat(recoverySignal); err == nil {
		return "standby"
	} else if !os.IsNotExist(err) {
		return "unknown"
	}

	// Check for recovery.conf (PG11 and earlier)
	recoveryConf := dataDir + "/recovery.conf"
	if _, err := os.Stat(recoveryConf); err == nil {
		return "standby"
	} else if !os.IsNotExist(err) {
		return "unknown"
	}

	// No recovery signals found = primary
	return "primary"
}

// calculateVersionNum converts PostgreSQL major version to version number format.
// PG 10+: major * 10000 (e.g., 17 -> 170000)
// PG 9.x: major * 10000 (simplified, no minor version info)
func calculateVersionNum(version int) int {
	return version * 10000
}

// calculateVersionNumFromString converts a PostgreSQL version string to version number format.
// Examples:
//
//	"17"   -> 170000
//	"9.6"  -> 90600
//	"12.4" -> 120000
//
// Returns 0 if parsing fails.
func calculateVersionNumFromString(version string) int {
	major, minor := parseVersionParts(version)
	if major == 0 {
		return 0
	}
	if major >= 10 {
		return major * 10000
	}
	return major*10000 + minor*100
}

func parseVersionParts(version string) (major int, minor int) {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0, 0
	}
	version = strings.TrimPrefix(strings.ToLower(version), "pg")
	version = strings.TrimSpace(version)
	if version == "" {
		return 0, 0
	}
	parts := strings.Split(version, ".")
	major, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	return major, minor
}

// parseConnectionInfoOutput parses the output from the connection info query.
// Expected format: "connections|max_connections\n"
// Returns connections, max_connections, and any parsing error.
func parseConnectionInfoOutput(output string) (connections, maxConnections int, err error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, 0, fmt.Errorf("empty output")
	}

	parts := strings.Split(output, "|")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected output format: %q", output)
	}

	connections, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("cannot parse connections: %w", err)
	}

	maxConnections, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("cannot parse max_connections: %w", err)
	}

	return connections, maxConnections, nil
}

// collectConnectionInfo queries PostgreSQL for current connection count and max_connections.
// Uses psql to execute query with DBSU privileges.
// Returns connections, max_connections, and error if query fails.
// Failure is graceful - caller should ignore error and omit fields.
func collectConnectionInfo(port int, dbsu string) (connections, maxConnections int, err error) {
	if port <= 0 {
		return 0, 0, fmt.Errorf("invalid port: %d", port)
	}

	// SQL query to get connection info
	query := `SELECT count(*) as conn, (SELECT setting::int FROM pg_settings WHERE name='max_connections') as max_conn FROM pg_stat_activity`

	// Build psql command
	args := []string{
		"psql",
		"-p", strconv.Itoa(port),
		"-d", "postgres",
		"-tAc", query,
	}

	// Execute via DBSU
	output, err := utils.DBSUCommandOutput(dbsu, args)
	if err != nil {
		logrus.Debugf("failed to collect connection info: %v", err)
		return 0, 0, err
	}

	return parseConnectionInfoOutput(output)
}

// checkFileExistsAsDBSU checks if a file exists, potentially using DBSU privilege.
// Returns (exists, error). Error indicates an inconclusive check (e.g., permission issues).
func checkFileExistsAsDBSU(dbsu, path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	}

	// If not root and dbsu is specified, try with sudo
	if dbsu != "" && os.Geteuid() != 0 {
		args := []string{}
		if os.Getenv("PIG_NON_INTERACTIVE") != "" {
			args = append(args, "-n")
		}
		args = append(args, "-u", dbsu, "test", "-f", path)
		cmd := exec.Command("sudo", args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return true, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			if strings.TrimSpace(string(output)) == "" {
				return false, nil
			}
		}
		return false, fmt.Errorf("dbsu check failed: %v", err)
	}

	return false, fmt.Errorf("permission denied")
}

// PatroniListEntry represents a member entry from patronictl list JSON output.
type PatroniListEntry struct {
	Cluster string `json:"Cluster"`
	Member  string `json:"Member"`
	Host    string `json:"Host"`
	Role    string `json:"Role"`
	State   string `json:"State"`
	TL      int    `json:"TL"`
	LagInMB *int   `json:"Lag in MB"`
}

// collectPatroniContext collects Patroni cluster information
func collectPatroniContext() *PatroniContext {
	ctx := &PatroniContext{Available: false}

	// Check if patronictl is installed
	if _, err := exec.LookPath("patronictl"); err != nil {
		logrus.Debug("patronictl not found in PATH")
		return ctx
	}

	ctx.Available = true

	// Try to get cluster info from config file (may fail due to permissions)
	clusterName := getPatroniClusterName()
	if clusterName != "" {
		ctx.Cluster = clusterName
	}

	// Check if patroni process is running
	ctx.Running = isPatroniRunning()
	if ctx.Running {
		ctx.State = "running"
		// Get detailed runtime info from patronictl
		info := getPatroniRuntimeInfo()
		if info != nil {
			if info.Role != "" {
				ctx.Role = info.Role
			}
			if info.State != "" {
				ctx.State = info.State
			}
			if info.Timeline > 0 {
				ctx.Timeline = info.Timeline
			}
			if info.Lag != "" {
				ctx.Lag = info.Lag
			}
			// If cluster name wasn't obtained from config, use from runtime
			if ctx.Cluster == "" && info.Cluster != "" {
				ctx.Cluster = info.Cluster
			}
		}
	}

	return ctx
}

// getPatroniClusterName reads the cluster name (scope) from patroni config file.
// Returns empty string if config cannot be read or parsed.
func getPatroniClusterName() string {
	// Try direct read first
	content, err := os.ReadFile(patroni.DefaultConfigPath)
	if err != nil {
		logrus.Debugf("cannot read patroni config directly: %v", err)
		// Try with DBSU privilege
		dbsu := utils.GetDBSU("")
		contentStr, err := utils.DBSUCommandOutput(dbsu, []string{"cat", patroni.DefaultConfigPath})
		if err != nil {
			logrus.Debugf("cannot read patroni config as DBSU: %v", err)
			return ""
		}
		content = []byte(contentStr)
	}

	// Simple YAML parsing for scope (cluster name)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "scope:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				cluster := strings.TrimSpace(parts[1])
				cluster = strings.Trim(cluster, "\"'")
				return cluster
			}
		}
	}

	return ""
}

// isPatroniRunning checks if patroni process is running.
// Uses systemctl first, falls back to pgrep.
func isPatroniRunning() bool {
	// Method A: systemctl is-active
	cmd := exec.Command("systemctl", "is-active", "--quiet", "patroni")
	if err := cmd.Run(); err == nil {
		return true
	}

	// Method B: pgrep
	cmd = exec.Command("pgrep", "-f", "patroni")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// patroniRuntimeInfo holds runtime information from patronictl list.
type patroniRuntimeInfo struct {
	Cluster  string
	Role     string
	State    string
	Timeline int
	Lag      string
}

// getPatroniRuntimeInfo executes patronictl list to get runtime status.
// Returns nil if command fails or current host not found.
func getPatroniRuntimeInfo() *patroniRuntimeInfo {
	dbsu := utils.GetDBSU("")
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		logrus.Debug("patronictl not found in PATH")
		return nil
	}
	args := []string{binPath, "-c", patroni.DefaultConfigPath, "list", "-f", "json"}

	output, err := utils.DBSUCommandOutput(dbsu, args)
	if err != nil {
		logrus.Debugf("patronictl list failed: %v", err)
		return nil
	}

	var entries []PatroniListEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		logrus.Debugf("failed to parse patronictl list output: %v", err)
		return nil
	}

	if len(entries) == 0 {
		logrus.Debug("patronictl list returned empty result")
		return nil
	}

	identifiers := hostIdentifiers()
	// Find entry matching current host identifiers
	for _, entry := range entries {
		if identifierMatches(identifiers, entry.Member) || identifierMatches(identifiers, entry.Host) {
			info := &patroniRuntimeInfo{
				Cluster:  strings.TrimSpace(entry.Cluster),
				Role:     normalizePatroniRole(entry.Role), // leader, replica, standby_leader
				State:    entry.State,
				Timeline: entry.TL,
			}
			if entry.LagInMB != nil {
				info.Lag = fmt.Sprintf("%d MB", *entry.LagInMB)
			}
			return info
		}
	}

	return nil
}

func normalizePatroniRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return role
	}
	parts := strings.Fields(role)
	return strings.Join(parts, "_")
}

func hostIdentifiers() map[string]struct{} {
	ids := make(map[string]struct{})
	hostname, _ := os.Hostname()
	if hostname != "" {
		lower := strings.ToLower(hostname)
		ids[lower] = struct{}{}
		if idx := strings.Index(lower, "."); idx > 0 {
			ids[lower[:idx]] = struct{}{}
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ids
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := extractIP(addr)
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ids[ip.String()] = struct{}{}
		}
	}
	return ids
}

func extractIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}

func identifierMatches(ids map[string]struct{}, value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if _, ok := ids[value]; ok {
		return true
	}
	if idx := strings.Index(value, "."); idx > 0 {
		_, ok := ids[value[:idx]]
		return ok
	}
	return false
}

func readPgVersionString(cfg *postgres.Config, dataDir string) string {
	if dataDir == "" {
		return ""
	}
	dbsu := postgres.GetDbSU(cfg)
	pgVersionFile := filepath.Join(dataDir, "PG_VERSION")
	output, err := utils.DBSUCommandOutput(dbsu, []string{"cat", pgVersionFile})
	if err != nil {
		logrus.Debugf("failed to read PG_VERSION: %v", err)
		return ""
	}
	return strings.TrimSpace(output)
}

// collectPgBackRestContext collects pgBackRest backup information
func collectPgBackRestContext() *PgBackRestContext {
	ctx := &PgBackRestContext{Available: false}

	// Check if pgbackrest is installed
	if _, err := exec.LookPath("pgbackrest"); err != nil {
		logrus.Debug("pgbackrest not found in PATH")
		return ctx
	}

	ctx.Available = true

	// Try to get info using pgbackrest.InfoResult
	cfg := &pgbackrest.Config{}
	opts := &pgbackrest.InfoOptions{}
	result := pgbackrest.InfoResult(cfg, opts)

	if result == nil {
		logrus.Debug("pgbackrest info failed or returned nil result")
		ctx.Configured = false
		return ctx
	}
	if result.Code == output.CodePbConfigNotFound || result.Code == output.CodePbStanzaNotFound {
		ctx.Configured = false
		return ctx
	}

	// Config exists, even if info command fails
	ctx.Configured = true

	// Extract data from result
	if result.Data == nil {
		return ctx
	}

	// Handle single stanza result
	if infoData, ok := result.Data.(*pgbackrest.PbInfoResultData); ok {
		ctx.Stanza = infoData.Stanza
		ctx.BackupCount = infoData.BackupCount
		if len(infoData.Backups) > 0 {
			// Get the most recent backup (last in the sorted list)
			lastBackup := infoData.Backups[len(infoData.Backups)-1]
			ctx.LastBackup = lastBackup.Label
			ctx.LastBackupTime = lastBackup.TimestampStop
		}
	}

	// Handle multiple stanzas result (array)
	if infoDataList, ok := result.Data.([]*pgbackrest.PbInfoResultData); ok && len(infoDataList) > 0 {
		// Use the first stanza for context
		first := infoDataList[0]
		ctx.Stanza = first.Stanza
		ctx.BackupCount = first.BackupCount
		if len(first.Backups) > 0 {
			lastBackup := first.Backups[len(first.Backups)-1]
			ctx.LastBackup = lastBackup.Label
			ctx.LastBackupTime = lastBackup.TimestampStop
		}
	}

	return ctx
}

// collectExtensionsContext collects installed extensions information
func collectExtensionsContext() *ExtensionsContext {
	ctx := &ExtensionsContext{Available: false}

	// Check if ext.Active is available
	if ext.Active == nil {
		logrus.Debug("ext.Active is nil, no PostgreSQL installation active")
		return ctx
	}

	// Try to scan extensions
	if err := ext.Active.ScanExtensions(); err != nil {
		logrus.Debugf("failed to scan extensions: %v", err)
		return ctx
	}

	ctx.Available = true
	ctx.InstalledCount = len(ext.Active.Extensions)

	// Collect extension names
	for _, extInstall := range ext.Active.Extensions {
		if extInstall != nil {
			ctx.Extensions = append(ctx.Extensions, extInstall.ExtName())
		}
	}

	return ctx
}
