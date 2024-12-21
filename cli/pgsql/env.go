package pgsql

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// the active PostgreSQL installation in the PATH
	Active *PostgresInstallation

	// all PostgreSQL installations in the system
	Installs map[int]*PostgresInstallation

	// the major versions of PostgreSQL to be detected
	PostgresActiveMajorVersions = []int{17, 16, 15, 14, 13}

	// the search paths for PostgreSQL installations
	PostgresElSearchPath = []string{
		"/usr/pgsql-%s/bin/pg_config", // RHEL/CentOS
		// "/usr/local/pgsql-%s/bin/pg_config",    // self-compiled
		// "/opt/postgresql-%s/bin/pg_config",     // other possible locations
	}
	PostgresDebianSearchPath = []string{
		"/usr/lib/postgresql/%s/bin/pg_config", // Debian/Ubuntu
		// "/usr/local/pgsql-%s/bin/pg_config",    // self-compiled
		// "/opt/postgresql-%s/bin/pg_config",     // other possible locations
	}
	// the regex for parsing the PostgreSQL version
	PostgresVersionRegex = regexp.MustCompile(`PostgreSQL (\d+)\.(\d+)`)

	sqlLessExtensions = map[string]string{
		"plan_filter":            "filter statements by their execution plans.",
		"pg_checksums":           "Activate/deactivate/verify checksums in offline Postgres clusters",
		"pg_crash":               "Send random signals to random processes",
		"vacuumlo":               "utility program that will remove any orphaned large objects from a PostgreSQL database",
		"oid2name":               "utility program that helps administrators to examine the file structure used by PostgreSQL",
		"basic_archive":          "an example of an archive module",
		"basebackup_to_shell":    "adds a custom basebackup target called shell",
		"bgw_replstatus":         "Small PostgreSQL background worker to report whether a node is a replication master or standby",
		"pg_relusage":            "Log all the queries that reference a particular column",
		"auto_explain":           "Provides a means for logging execution plans of slow statements automatically",
		"passwordcheck_cracklib": "Strengthen PostgreSQL user password checks with cracklib",
		"supautils":              "Extension that secures a cluster on a cloud environment",
		"pg_snakeoil":            "The PostgreSQL Antivirus",
		"pgextwlist":             "PostgreSQL Extension Whitelisting",
		"auth_delay":             "pause briefly before reporting authentication failure",
		"passwordcheck":          "checks user passwords and reject weak password",
		"pg_statement_rollback":  "Server side rollback at statement level for PostgreSQL like Oracle or DB2",
		"wal2json":               "Changing data capture in JSON format",
		"wal2mongo":              "PostgreSQL logical decoding output plugin for MongoDB",
		"decoderbufs":            "Logical decoding plugin that delivers WAL stream changes using a Protocol Buffer format",
		"decoder_raw":            "Output plugin for logical replication in Raw SQL format",
		"test_decoding":          "SQL-based test/example module for WAL logical decoding",
	}
)

// PostgresInstallation stores information about a PostgreSQL installation
type PostgresInstallation struct {
	Version       string
	MajorVersion  int
	MinorVersion  int
	BinPath       string
	LibPath       string
	SharePath     string
	IncludePath   string
	ExtensionPath string
	Extensions    []Extension
	SharedLibs    []SharedLib
	UnmatchedLibs []SharedLib
}

// Extension stores information about a PostgreSQL extension
type Extension struct {
	Name        string            // Extension name
	Version     string            // Extension version
	Description string            // Extension description
	InstalledAt time.Time         // Installation time (from control file)
	Meta        map[string]string // Metadata
	Library     *SharedLib        // Associated shared library
}

// SharedLib stores information about a shared library
type SharedLib struct {
	Name      string    // Library name
	Path      string    // Full path
	Size      int64     // File size (bytes)
	CreatedAt time.Time // Creation time
}

func (p *PostgresInstallation) String() string {
	return fmt.Sprintf("PostgreSQL %d.%d: %s",
		p.MajorVersion,
		p.MinorVersion,
		p.BinPath,
	)
}

// DetectPostgres detects the active PostgreSQL installation
func DetectPostgres() error {
	install, err := detectActiveInstall()
	if err != nil {
		return fmt.Errorf("failed to detect PostgreSQL: %v", err)
	}
	if err := install.ScanExtensions(); err != nil {
		return fmt.Errorf("failed to scan extensions: %v", err)
	}
	Active = install
	return nil
}

// DetectInstalledPostgres detects all installed PostgreSQL versions on the system
func DetectInstalledPostgres() error {
	Installs = make(map[int]*PostgresInstallation)
	var searchPath []string
	if config.OSType == config.DistroDEB {
		searchPath = PostgresDebianSearchPath
	} else {
		searchPath = PostgresElSearchPath
	}

	if err := DetectPostgres(); err == nil && Active != nil {
		Installs[Active.MajorVersion] = Active
	}

	for _, v := range PostgresActiveMajorVersions {
		if _, exists := Installs[v]; exists {
			continue
		}

		for _, pattern := range searchPath {
			verStr := strconv.Itoa(v)
			pgConfigPath := fmt.Sprintf(pattern, verStr)
			if _, err := os.Stat(pgConfigPath); err != nil {
				continue
			}

			install := &PostgresInstallation{}
			if err := install.detectFromConfig(pgConfigPath); err != nil {
				continue
			}

			if err := install.ScanExtensions(); err != nil {
				logrus.Debugf("failed to scan extensions for PostgreSQL %d: %v", v, err)
			}

			Installs[install.MajorVersion] = install
			logrus.Debugf("found PostgreSQL %d at %s", v, pgConfigPath)
			break // Move to the next version after finding a working installation
		}
	}

	return nil
}

// detectFromConfig retrieves installation information from the specified pg_config path
func (p *PostgresInstallation) detectFromConfig(pgConfigPath string) error {
	paths := map[string]*string{
		"--version":    &p.Version,
		"--bindir":     &p.BinPath,
		"--libdir":     &p.LibPath,
		"--sharedir":   &p.SharePath,
		"--includedir": &p.IncludePath,
		"--pkglibdir":  &p.ExtensionPath,
	}

	for param, target := range paths {
		cmd := exec.Command(pgConfigPath, param)
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to execute pg_config %s: %v", param, err)
		}
		*target = strings.TrimSpace(string(output))
	}

	if matches := PostgresVersionRegex.FindStringSubmatch(p.Version); len(matches) >= 3 {
		p.MajorVersion, _ = strconv.Atoi(matches[1])
		p.MinorVersion, _ = strconv.Atoi(matches[2])
	}

	return nil
}

// ScanExtensions scans PostgreSQL extensions
func (p *PostgresInstallation) ScanExtensions() error {
	// Track matched shared libraries
	matchedLibs := make(map[string]struct{})

	// Scan shared libraries
	sharedLibs := make(map[string]SharedLib)
	if err := p.scanSharedLibs(sharedLibs); err != nil {
		return fmt.Errorf("failed to scan shared libraries: %v", err)
	}

	extensionsPath := filepath.Join(p.SharePath, "extension")
	entries, err := os.ReadDir(extensionsPath)
	if err != nil {
		return fmt.Errorf("failed to read extensions directory: %v", err)
	}

	controlFiles := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".control") {
			ext := Extension{
				Name: strings.TrimSuffix(entry.Name(), ".control"),
			}

			if err := p.parseControlFile(&ext); err != nil {
				continue
			}

			if lib, exists := sharedLibs[ext.Name]; exists {
				ext.Library = &lib
				if ext.InstalledAt.IsZero() {
					ext.InstalledAt = lib.CreatedAt
				}
				matchedLibs[ext.Name] = struct{}{} // Record match
			}

			p.Extensions = append(p.Extensions, ext)
			controlFiles[ext.Name] = struct{}{}
		}
	}

	// Handle extensions without control files
	for name, description := range sqlLessExtensions {
		if _, known := controlFiles[name]; !known {
			if lib, exists := sharedLibs[name]; exists {
				ext := Extension{
					Name:        name,
					Description: description,
					Library:     &lib,
					InstalledAt: lib.CreatedAt,
				}
				p.Extensions = append(p.Extensions, ext)
				matchedLibs[name] = struct{}{} // Record match
			}
		}
	}

	// Find unmatched shared libraries
	var unmatchedLibs []SharedLib
	for name, lib := range sharedLibs {
		if _, matched := matchedLibs[name]; !matched {
			unmatchedLibs = append(unmatchedLibs, lib)
		}
	}

	// Sort unmatched shared libraries by name
	sort.Slice(unmatchedLibs, func(i, j int) bool {
		return unmatchedLibs[i].Name < unmatchedLibs[j].Name
	})

	p.UnmatchedLibs = unmatchedLibs
	p.SharedLibs = unmatchedLibs // Maintain backward compatibility

	return nil
}

// parseControlFile parses the control file of an extension
func (p *PostgresInstallation) parseControlFile(ext *Extension) error {
	controlPath := filepath.Join(p.SharePath, "extension", ext.Name+".control")
	file, err := os.Open(controlPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get the creation time of the control file
	if info, err := file.Stat(); err == nil {
		ext.InstalledAt = info.ModTime()
	}

	// Initialize metadata map
	ext.Meta = make(map[string]string)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// If the line contains a comment, only take the part before the comment
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "'")

		switch key {
		case "default_version":
			ext.Version = value
		case "comment":
			ext.Description = value
		default:
			ext.Meta[key] = value
		}
	}

	return scanner.Err()
}

// scanSharedLibs scans shared library files
func (p *PostgresInstallation) scanSharedLibs(libs map[string]SharedLib) error {
	libPaths := []string{
		p.ExtensionPath,
		filepath.Join(p.LibPath, "postgresql"),
	}

	for _, libPath := range libPaths {
		entries, err := os.ReadDir(libPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".so") {
				name := strings.TrimSuffix(entry.Name(), ".so")
				fullPath := filepath.Join(libPath, entry.Name())

				info, err := entry.Info()
				if err != nil {
					continue
				}

				lib := SharedLib{
					Name:      name,
					Path:      fullPath,
					Size:      info.Size(),
					CreatedAt: info.ModTime(),
				}

				p.SharedLibs = append(p.SharedLibs, lib)
				libs[name] = lib
			}
		}
	}

	return nil
}

// detectActiveInstall detects the active PostgreSQL installation
func detectActiveInstall() (*PostgresInstallation, error) {
	pgConfig, err := exec.LookPath("pg_config")
	if err != nil {
		return nil, fmt.Errorf("pg_config not found in PATH: %v", err)
	}
	install := &PostgresInstallation{}
	paths := map[string]*string{
		"--version":    &install.Version,
		"--bindir":     &install.BinPath,
		"--libdir":     &install.LibPath,
		"--sharedir":   &install.SharePath,
		"--includedir": &install.IncludePath,
		"--pkglibdir":  &install.ExtensionPath,
	}

	for param, target := range paths {
		cmd := exec.Command(pgConfig, param)
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to execute pg_config %s: %v", param, err)
		}
		*target = strings.TrimSpace(string(output))
	}

	versionRegex := regexp.MustCompile(`PostgreSQL (\d+)\.(\d+)`)
	matches := versionRegex.FindStringSubmatch(install.Version)
	if len(matches) >= 3 {
		install.MajorVersion, _ = strconv.Atoi(matches[1])
		install.MinorVersion, _ = strconv.Atoi(matches[2])
	}
	return install, nil
}
