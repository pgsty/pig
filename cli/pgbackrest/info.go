package pgbackrest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// JSON Parsing Structures for pgBackRest info --output=json
// ============================================================================

// PgBackRestInfo represents the top-level JSON array element from pgbackrest info
type PgBackRestInfo struct {
	Archive []ArchiveInfo `json:"archive"`
	Backup  []BackupInfo  `json:"backup"`
	Cipher  string        `json:"cipher"`
	DB      []DBInfo      `json:"db"`
	Name    string        `json:"name"`
	Repo    []RepoInfo    `json:"repo"`
	Status  StatusInfo    `json:"status"`
}

// ArchiveInfo represents WAL archive information
type ArchiveInfo struct {
	Database DBRef  `json:"database"`
	ID       string `json:"id"`
	Max      string `json:"max"`
	Min      string `json:"min"`
}

// BackupInfo represents a single backup entry
type BackupInfo struct {
	Annotation map[string]string `json:"annotation"`
	Archive    BackupArchive     `json:"archive"`
	Backrest   BackrestVersion   `json:"backrest"`
	Database   DBRef             `json:"database"`
	Error      bool              `json:"error"`
	Info       BackupSizeInfo    `json:"info"`
	Label      string            `json:"label"`
	LSN        LSNRange          `json:"lsn"`
	Prior      *string           `json:"prior"`
	Reference  []string          `json:"reference"`
	Timestamp  TimestampRange    `json:"timestamp"`
	Type       string            `json:"type"`
}

// BackupArchive represents the WAL segment range for a backup
type BackupArchive struct {
	Start string `json:"start"`
	Stop  string `json:"stop"`
}

// BackrestVersion represents pgbackrest version info
type BackrestVersion struct {
	Format  int    `json:"format"`
	Version string `json:"version"`
}

// DBRef represents a database reference
type DBRef struct {
	ID      int `json:"id"`
	RepoKey int `json:"repo-key"`
}

// BackupSizeInfo represents backup size information
type BackupSizeInfo struct {
	Delta      int64          `json:"delta"`
	Repository RepositoryInfo `json:"repository"`
	Size       int64          `json:"size"`
}

// RepositoryInfo represents repository-specific size info
type RepositoryInfo struct {
	Delta    int64 `json:"delta"`
	DeltaMap int64 `json:"delta-map"`
	Size     int64 `json:"size"`
	SizeMap  int64 `json:"size-map"`
}

// LSNRange represents a start/stop LSN range
type LSNRange struct {
	Start string `json:"start"`
	Stop  string `json:"stop"`
}

// TimestampRange represents start/stop timestamps (Unix epoch)
type TimestampRange struct {
	Start int64 `json:"start"`
	Stop  int64 `json:"stop"`
}

// DBInfo represents database information
type DBInfo struct {
	ID       int    `json:"id"`
	RepoKey  int    `json:"repo-key"`
	SystemID int64  `json:"system-id"`
	Version  string `json:"version"`
}

// RepoInfo represents repository status
type RepoInfo struct {
	Cipher string         `json:"cipher"`
	Key    int            `json:"key"`
	Status RepoStatusInfo `json:"status"`
}

// RepoStatusInfo represents repository status details
type RepoStatusInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// StatusInfo represents overall status
type StatusInfo struct {
	Code    int        `json:"code"`
	Lock    LockStatus `json:"lock"`
	Message string     `json:"message"`
}

// LockStatus represents lock status for backup/restore
type LockStatus struct {
	Backup  LockHeld `json:"backup"`
	Restore LockHeld `json:"restore"`
}

// LockHeld represents whether a lock is held
type LockHeld struct {
	Held bool `json:"held"`
}

// ============================================================================
// Info Command Implementation
// ============================================================================

// InfoOptions holds options for the info command.
type InfoOptions struct {
	Output string // Output format: text, json (passed to pgbackrest)
	Set    string // Specific backup set to show
	Raw    bool   // Raw output mode (pass through pgbackrest output)
}

// Info displays backup repository information.
func Info(cfg *Config, opts *InfoOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	// Raw mode: pass through to pgbackrest directly
	if opts.Raw {
		var args []string
		if opts.Output != "" {
			args = append(args, "--output="+opts.Output)
		}
		if opts.Set != "" {
			args = append(args, "--set="+opts.Set)
		}
		return RunPgBackRestRaw(effCfg, "info", args, true)
	}

	// Detailed mode: fetch JSON and parse
	// Suppress console logs to keep JSON output clean.
	args := []string{"--output=json", "--log-level-console=error"}
	if opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}

	output, err := RunPgBackRestOutput(effCfg, "info", args)
	if err != nil {
		return fmt.Errorf("failed to get backup info: %w", err)
	}

	// Parse JSON output
	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(output), &infos); err != nil {
		// If JSON parsing fails, fall back to raw output
		fmt.Println(output)
		return nil
	}

	// Display detailed info for each stanza
	for _, info := range infos {
		printDetailedInfo(&info)
	}

	return nil
}

// printDetailedInfo prints detailed backup information for a stanza
func printDetailedInfo(info *PgBackRestInfo) {
	// Print stanza header with encryption info
	fmt.Printf("Stanza: %s", info.Name)
	if info.Cipher != "" && info.Cipher != "none" {
		fmt.Printf(" (encrypted: %s)", info.Cipher)
	}
	fmt.Println()

	// Print status
	statusIcon := "✓"
	if info.Status.Code != 0 {
		statusIcon = "✗"
	}
	fmt.Printf("Status: %s %s\n", statusIcon, info.Status.Message)

	// Print DB info
	if len(info.DB) > 0 {
		db := info.DB[0]
		fmt.Printf("Database: PostgreSQL %s (system-id: %d)\n", db.Version, db.SystemID)
	}

	// No backups case
	if len(info.Backup) == 0 {
		fmt.Println("\nNo backups available.")
		return
	}

	// Sort backups by start timestamp (ascending)
	backups := make([]BackupInfo, len(info.Backup))
	copy(backups, info.Backup)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.Start < backups[j].Timestamp.Start
	})

	// Get first and last backups
	firstBackup := backups[0]
	lastBackup := backups[len(backups)-1]
	firstBackupTime := time.Unix(firstBackup.Timestamp.Start, 0)
	lastBackupTime := time.Unix(lastBackup.Timestamp.Stop, 0)

	// 1. Print WAL archive range
	if len(info.Archive) > 0 {
		fmt.Println()
		fmt.Println("WAL Archive:")
		for _, archive := range info.Archive {
			if archive.Min == archive.Max {
				fmt.Printf("  %s\n", archive.Min)
			} else {
				fmt.Printf("  %s - %s\n", archive.Min, archive.Max)
			}
		}
	}

	// 2. Print LSN range from backups (PostgreSQL style with leading zeros)
	fmt.Println()
	fmt.Println("LSN Range:")
	fmt.Printf("  Start: %s (from: %s)\n", formatLSNPadded(firstBackup.LSN.Start), firstBackup.Label)
	fmt.Printf("  Stop:  %s (from: %s)\n", formatLSNPadded(lastBackup.LSN.Stop), lastBackup.Label)

	// 3. Calculate and print recovery window
	windowDuration := lastBackupTime.Sub(firstBackupTime)

	// Calculate max label width for alignment
	maxLabelLen := len(firstBackup.Label)
	if len(lastBackup.Label) > maxLabelLen {
		maxLabelLen = len(lastBackup.Label)
	}

	fmt.Println()
	fmt.Printf("Recovery Window: %s\n", formatDurationCompactLong(windowDuration))
	fmt.Printf("  First Backup: %-*s  %s   %s\n",
		maxLabelLen, firstBackup.Label,
		firstBackupTime.UTC().Format("2006-01-02 15:04:05"),
		formatLocalTimePITRQuoted(firstBackupTime))
	fmt.Printf("  Last  Backup: %-*s  %s   %s\n",
		maxLabelLen, lastBackup.Label,
		lastBackupTime.UTC().Format("2006-01-02 15:04:05"),
		formatLocalTimePITRQuoted(lastBackupTime))

	// 4. Print backup list table
	fmt.Println()
	printBackupTable(backups)
}

// formatLocalTimePITR formats time in local timezone with ISO format for PITR
// Format: 2025-01-01 12:00:00+08
func formatLocalTimePITR(t time.Time) string {
	local := t.Local()
	_, offset := local.Zone()
	hours := offset / 3600
	mins := (offset % 3600) / 60
	if mins < 0 {
		mins = -mins
	}
	if mins == 0 {
		return fmt.Sprintf("%s%+03d", local.Format("2006-01-02 15:04:05"), hours)
	}
	return fmt.Sprintf("%s%+03d:%02d", local.Format("2006-01-02 15:04:05"), hours, mins)
}

// formatLocalTimePITRQuoted formats time in local timezone with quotes for PITR
// Format: "2025-01-01 12:00:00+08"
func formatLocalTimePITRQuoted(t time.Time) string {
	return fmt.Sprintf("\"%s\"", formatLocalTimePITR(t))
}

// formatDurationCompactLong formats duration in compact but readable form
// Format: 1d 5h 46m
func formatDurationCompactLong(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	return strings.Join(parts, " ")
}

// formatLSNPadded formats LSN with leading zeros preserved (PostgreSQL style)
// Input: "0/3000028" -> Output: "0/03000028" (padded to 8 hex chars after /)
func formatLSNPadded(lsn string) string {
	parts := strings.Split(lsn, "/")
	if len(parts) != 2 {
		return lsn
	}
	// Pad each part: first part to at least 1 char, second part to 8 chars
	return fmt.Sprintf("%s/%08s", parts[0], parts[1])
}

// printBackupTable prints a formatted table of backups
func printBackupTable(backups []BackupInfo) {
	// Calculate max widths for dynamic columns
	maxNameLen := 11 // "Backup Name"
	maxPriorLen := 5 // "Prior"
	for _, b := range backups {
		if len(b.Label) > maxNameLen {
			maxNameLen = len(b.Label)
		}
		if b.Prior != nil && len(*b.Prior) > maxPriorLen {
			maxPriorLen = len(*b.Prior)
		}
	}

	// Print header with dynamic column widths
	fmt.Printf("%-4s  %-*s  %-*s  %2s  %5s  %5s  %5s  %-22s  %-22s  %-23s  %s\n",
		"Type", maxNameLen, "Backup Name", maxPriorLen, "Prior", "Δt", "Size", "Data", "Repo", "Start", "Stop", "LSN Range", "WAL")
	fmt.Printf("%-4s  %-*s  %-*s  %2s  %5s  %5s  %5s  %-22s  %-22s  %-23s  %s\n",
		"----", maxNameLen, strings.Repeat("-", maxNameLen), maxPriorLen, strings.Repeat("-", maxPriorLen),
		"--", "-----", "----", "----", strings.Repeat("-", 22), strings.Repeat("-", 22), strings.Repeat("-", 23), "---")

	for _, b := range backups {
		// Format type (full uppercase name)
		typeStr := "?"
		switch b.Type {
		case "full":
			typeStr = "FULL"
		case "diff":
			typeStr = "DIFF"
		case "incr":
			typeStr = "INCR"
		}

		// Prior (full, no truncation)
		prior := "-"
		if b.Prior != nil && *b.Prior != "" {
			prior = *b.Prior
		}

		// Format duration (compact)
		duration := time.Duration(b.Timestamp.Stop-b.Timestamp.Start) * time.Second
		durationStr := formatDurationCompact(duration)

		// Format sizes (compact, 1 decimal max)
		sizeStr := formatBytesCompact(b.Info.Size)
		deltaDataStr := formatBytesCompact(b.Info.Delta)
		deltaRepoStr := formatBytesCompact(b.Info.Repository.Delta)

		// Format start/stop times (local, PITR format)
		startTime := time.Unix(b.Timestamp.Start, 0)
		stopTime := time.Unix(b.Timestamp.Stop, 0)
		startStr := formatLocalTimePITR(startTime)
		stopStr := formatLocalTimePITR(stopTime)

		// Format LSN range (padded, PostgreSQL style, using " - " separator)
		lsnStart := formatLSNPadded(b.LSN.Start)
		lsnStop := formatLSNPadded(b.LSN.Stop)
		lsnRange := lsnStart
		if lsnStop != "" && lsnStop != lsnStart {
			lsnRange = fmt.Sprintf("%s - %s", lsnStart, lsnStop)
		}

		// Format WAL (full segment name, show start only if start==stop)
		walStr := b.Archive.Start
		if b.Archive.Stop != "" && b.Archive.Stop != b.Archive.Start {
			walStr = fmt.Sprintf("%s - %s", b.Archive.Start, b.Archive.Stop)
		}

		fmt.Printf("%-4s  %-*s  %-*s  %2s  %5s  %5s  %5s  %-22s  %-22s  %-23s  %s\n",
			typeStr, maxNameLen, b.Label, maxPriorLen, prior, durationStr, sizeStr, deltaDataStr, deltaRepoStr, startStr, stopStr, lsnRange, walStr)
	}
}

// formatDurationCompact formats duration in very compact form (e.g., "1s", "2m", "1h")
func formatDurationCompact(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm%d", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%d", hours, mins)
}

// formatBytesCompact formats bytes in compact form with max 1 decimal
func formatBytesCompact(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		v := float64(bytes) / TB
		if v >= 10 {
			return fmt.Sprintf("%.0fT", v)
		}
		return fmt.Sprintf("%.1fT", v)
	case bytes >= GB:
		v := float64(bytes) / GB
		if v >= 10 {
			return fmt.Sprintf("%.0fG", v)
		}
		return fmt.Sprintf("%.1fG", v)
	case bytes >= MB:
		v := float64(bytes) / MB
		if v >= 10 {
			return fmt.Sprintf("%.0fM", v)
		}
		return fmt.Sprintf("%.1fM", v)
	case bytes >= KB:
		v := float64(bytes) / KB
		if v >= 10 {
			return fmt.Sprintf("%.0fK", v)
		}
		return fmt.Sprintf("%.1fK", v)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// LsOptions holds options for the ls command.
type LsOptions struct {
	Type string // List type: backup, repo, stanza
}

// Ls lists resources in the backup repository.
func Ls(cfg *Config, opts *LsOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	switch opts.Type {
	case "", "backup":
		return RunPgBackRest(effCfg, "info", nil, true)
	case "repo":
		return listRepos(effCfg)
	case "stanza", "cluster", "cls":
		return listStanzas(effCfg)
	default:
		return fmt.Errorf("unknown list type: %s (use: backup, repo, stanza)", opts.Type)
	}
}

// listRepos parses config file and lists configured repositories.
// Uses DBSU privilege escalation if needed.
func listRepos(cfg *Config) error {
	content, err := readConfigFile(cfg.ConfigPath, cfg.DbSU)
	if err != nil {
		return fmt.Errorf("cannot read config file: %w", err)
	}

	repos := make(map[string]map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if matches := repoConfigRegex.FindStringSubmatch(line); matches != nil {
			repoName, key, value := matches[1], matches[2], matches[3]
			if repos[repoName] == nil {
				repos[repoName] = make(map[string]string)
			}
			repos[repoName][key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading config: %w", err)
	}

	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "No repositories configured")
		return nil
	}

	fmt.Printf("%-8s %-8s %s\n", "REPO", "TYPE", "PATH/ENDPOINT")
	fmt.Printf("%-8s %-8s %s\n", "----", "----", "-------------")

	// Print repos in order (repo1, repo2, ... up to repo10)
	for i := 1; i <= 10; i++ {
		repoName := fmt.Sprintf("repo%d", i)
		repo, ok := repos[repoName]
		if !ok {
			continue
		}

		repoType := repo["type"]
		if repoType == "" {
			repoType = "posix"
		}

		path := formatRepoPath(repoType, repo)
		fmt.Printf("%-8s %-8s %s\n", repoName, repoType, path)
	}

	return nil
}

// formatRepoPath formats the repository path based on type.
func formatRepoPath(repoType string, repo map[string]string) string {
	switch repoType {
	case "posix", "cifs":
		return repo["path"]
	case "s3":
		bucket := repo["s3-bucket"]
		if endpoint := repo["s3-endpoint"]; endpoint != "" {
			return fmt.Sprintf("s3://%s (endpoint: %s)", bucket, endpoint)
		}
		if region := repo["s3-region"]; region != "" {
			return fmt.Sprintf("s3://%s (%s)", bucket, region)
		}
		return fmt.Sprintf("s3://%s", bucket)
	case "azure":
		return fmt.Sprintf("azure://%s", repo["azure-container"])
	case "gcs":
		return fmt.Sprintf("gcs://%s", repo["gcs-bucket"])
	default:
		return repo["path"]
	}
}

// stanzaInfo holds parsed stanza information.
type stanzaInfo struct {
	Name   string
	PgPath string
	PgPort string
}

// listStanzas lists all stanzas in the config file.
// Uses DBSU privilege escalation if needed.
func listStanzas(cfg *Config) error {
	content, err := readConfigFile(cfg.ConfigPath, cfg.DbSU)
	if err != nil {
		return fmt.Errorf("cannot read config file: %w", err)
	}

	var stanzas []stanzaInfo
	var current *stanzaInfo

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if matches := sectionRegex.FindStringSubmatch(line); matches != nil {
			section := matches[1]
			if !strings.HasPrefix(section, "global") {
				if current != nil {
					stanzas = append(stanzas, *current)
				}
				current = &stanzaInfo{Name: section}
			} else {
				if current != nil {
					stanzas = append(stanzas, *current)
					current = nil
				}
			}
			continue
		}

		if current != nil {
			if matches := pgPathRegex.FindStringSubmatch(line); matches != nil {
				current.PgPath = strings.TrimSpace(matches[1])
			}
			if matches := pgPortRegex.FindStringSubmatch(line); matches != nil {
				current.PgPort = strings.TrimSpace(matches[1])
			}
		}
	}

	if current != nil {
		stanzas = append(stanzas, *current)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading config: %w", err)
	}

	if len(stanzas) == 0 {
		fmt.Fprintln(os.Stderr, "No stanzas configured")
		return nil
	}

	fmt.Printf("%-15s %-25s %s\n", "STANZA", "PG PATH", "PG PORT")
	fmt.Printf("%-15s %-25s %s\n", "------", "-------", "-------")
	for _, s := range stanzas {
		port := s.PgPort
		if port == "" {
			port = "5432"
		}
		fmt.Printf("%-15s %-25s %s\n", s.Name, s.PgPath, port)
	}

	return nil
}
