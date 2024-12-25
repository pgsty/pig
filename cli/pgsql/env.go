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

	"github.com/dustin/go-humanize"
	"github.com/sirupsen/logrus"
)

var (
	// Inited is a global flag to indicate if the PostgreSQL environment has been detected
	Inited = false

	// Active is the active PostgreSQL installation in the PATH
	Active *PostgresInstallation

	// Installs is all PostgreSQL installations in the system
	Installs map[int]*PostgresInstallation

	// PostgresActiveMajorVersions is the major versions of PostgreSQL to be detected
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

	// sqlLessExtensions is a map of extensions that do not have control files (so lib only)
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
		"pgoutput":               "Logical Replication output plugin",
		"test_decoding":          "SQL-based test/example module for WAL logical decoding",
		"safeupdate":             "Require criteria for UPDATE and DELETE",
		"sepgsql":                "label-based mandatory access control (MAC) based on SELinux security policy",
		"pg_failover_slots":      "PG Failover Slots extension",
	}

	// badCaseExtensions is a map of extensions that are known to be problematic and should be handled explicitly
	badCaseExtensions = map[string]bool{
		"address_standardizer-3":         true,
		"address_standardizer_data_us-3": true,
		"postgis-3":                      true,
		"postgis_raster-3":               true,
		"postgis_sfcgal-3":               true,
		"postgis_tiger_geocoder-3":       true,
		"postgis_topology-3":             true,
		"pg_proctab--0.0.10-compat":      true,
	}
)

// PostgresInstallation stores information about a PostgreSQL installation
type PostgresInstallation struct {
	Version       string
	MajorVersion  int
	MinorVersion  int
	BinPath       string
	ExtPath       string
	LibPath       string
	PgConfig      string
	Config        map[string]string
	Extensions    []*ExtensionCtrl
	SharedLibs    []*SharedLib
	UnmatchedLibs []*SharedLib
	UnmatchedExts []*ExtensionCtrl
	ExtMap        map[string]*ExtensionCtrl
	LibMap        map[string]*SharedLib
}

// Extension stores information about a PostgreSQL extension
type ExtensionCtrl struct {
	Name        string            // Extension name
	Version     string            // Extension version
	Description string            // Extension description
	Mtime       time.Time         // Installation time (from control file)
	Meta        map[string]string // Metadata
	Library     *SharedLib        // Associated shared library
}

// SharedLib stores information about a shared library
type SharedLib struct {
	Name    string         // Library name
	ExtName string         // Extension name (strip version suffix)
	Path    string         // Full path
	Size    int64          // File size (bytes)
	Mtime   time.Time      // Creation time
	Ext     *ExtensionCtrl // Associated extension
}

func (p *PostgresInstallation) String() string {
	return fmt.Sprintf("PostgreSQL %d.%d: %s",
		p.MajorVersion,
		p.MinorVersion,
		p.BinPath,
	)

}

func (pg *PostgresInstallation) PrintSummary() {
	fmt.Printf("PostgreSQL     :  %s\n", pg.Version)
	fmt.Printf("Binary Path    :  %s\n", pg.BinPath)
	fmt.Printf("Library Path   :  %s\n", pg.LibPath)
	fmt.Printf("PgConfig Path  :  %s\n", pg.PgConfig)
	fmt.Printf("Extension Path :  %s\n", pg.ExtPath)
}

func (e *ExtensionCtrl) LibName() string {
	// return human-readable size
	if e.Library == nil {
		return ""
	} else {
		return e.Library.Name + ".so"
	}
}

func (e *ExtensionCtrl) Size() string {
	// return human-readable size
	if e.Library == nil {
		return ""
	} else {
		return humanize.Bytes(uint64(e.Library.Size))
	}
}

// PostgresInstallSummary print the summary of PostgreSQL installation
func PostgresInstallSummary() {
	if !Inited {
		fmt.Printf("PostgreSQL Environment not initialized\n")
		return
	}

	// print installed PostgreSQL versions
	if len(Installs) > 0 {
		fmt.Printf("Installed:\n")
		for _, v := range Installs {
			if v == Active {
				fmt.Printf("* %-17s\t%s\n", fmt.Sprintf("%d.%d", v.MajorVersion, v.MinorVersion), v.PgConfig)
			}
		}
		for _, v := range Installs {
			if v != Active {
				fmt.Printf("- %-15s\t%s\n", fmt.Sprintf("%d.%d", v.MajorVersion, v.MinorVersion), v.PgConfig)
			}
		}
	} else {
		fmt.Println("No PostgreSQL installtion found")
	}

	// print active PostgreSQL detail
	if Active != nil {
		fmt.Printf("\nActive:\n")
		fmt.Printf("PG Version        :  %s\n", Active.Version)
		fmt.Printf("Config Path       :  %s\n", Active.PgConfig)
		fmt.Printf("Binary Path       :  %s\n", Active.BinPath)
		fmt.Printf("Library Path      :  %s\n", Active.LibPath)
		fmt.Printf("Extension Path    :  %s\n", Active.ExtPath)
		if len(Active.Extensions) > 0 {
			fmt.Printf("Extension Stat    :  Installed %d\n", len(Active.Extensions))
		}
	} else {
		fmt.Println("No PostgreSQL installation actiavated")
		fmt.Printf("PATH: %s\n", os.Getenv("PATH"))
	}
}

// GetPostgres returns the active PostgreSQL installation (via pg_config path or major version)
func GetPostgres(path string, ver int) (pg *PostgresInstallation, err error) {
	if path != "" {
		return DetectPostgresFromConfig(path)
	}
	if !Inited {
		err = DetectInstalledPostgres()
		if err != nil {
			return nil, err
		}
	}
	if ver != 0 {
		if pg, exists := Installs[ver]; exists {
			return pg, nil
		} else {
			return nil, fmt.Errorf("PostgreSQL version %d is not installed", ver)
		}
	}
	if Active == nil {
		return nil, fmt.Errorf("no active PostgreSQL installation detected")
	} else {
		return Active, nil
	}
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

	if err := DetectActivePostgres(); err == nil && Active != nil {
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
			install.PgConfig = pgConfigPath
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

	Inited = true
	return nil
}

// DetectPostgres detects the active PostgreSQL installation
func DetectActivePostgres() error {
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

// DetectPostgresFromConfig detects PostgreSQL installation from a specific pg_config path
func DetectPostgresFromConfig(pgConfigPath string) (*PostgresInstallation, error) {
	install := &PostgresInstallation{}
	if err := install.detectFromConfig(pgConfigPath); err != nil {
		return nil, fmt.Errorf("failed to detect PostgreSQL from %s: %v", pgConfigPath, err)
	}

	if err := install.ScanExtensions(); err != nil {
		return nil, fmt.Errorf("failed to scan extensions: %v", err)
	}

	return install, nil
}

// detectFromConfig retrieves installation information from the specified pg_config path
func (p *PostgresInstallation) detectFromConfig(pgConfigPath string) error {
	cmd := exec.Command(pgConfigPath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to execute pg_config: %v", err)
	}

	config := strings.TrimSpace(string(output))
	lines := strings.Split(config, "\n")
	configMap := make(map[string]string)

	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			configMap[key] = value
		}
	}

	if version, ok := configMap["VERSION"]; ok {
		p.Version = version
		if matches := PostgresVersionRegex.FindStringSubmatch(p.Version); len(matches) >= 3 {
			p.MajorVersion, _ = strconv.Atoi(matches[1])
			p.MinorVersion, _ = strconv.Atoi(matches[2])
		}
	}
	if bindir, ok := configMap["BINDIR"]; ok {
		p.BinPath = bindir
	}
	if libdir, ok := configMap["PKGLIBDIR"]; ok {
		p.LibPath = libdir
	}
	if sharedir, ok := configMap["SHAREDIR"]; ok {
		p.ExtPath = filepath.Join(sharedir, "extension")
	}

	return nil
}

// ScanExtensions scans PostgreSQL extensions
func (p *PostgresInstallation) ScanExtensions() error {
	if err := p.scanSharedLibs(); err != nil {
		return fmt.Errorf("failed to scan shared libraries: %v", err)
	}
	if err := p.scanExtensions(); err != nil {
		return fmt.Errorf("failed to scan extensions: %v", err)
	}
	if err := p.matchExtensionLibs(); err != nil {
		return fmt.Errorf("failed to match extension libs: %v", err)
	}
	var unmatchedExts []*ExtensionCtrl
	var unmatchedLibs []*SharedLib
	for _, ext := range p.Extensions {
		if ext.Library == nil {
			unmatchedExts = append(unmatchedExts, ext)
		}
	}
	for _, lib := range p.SharedLibs {
		if lib.Ext == nil {
			unmatchedLibs = append(unmatchedLibs, lib)
		}
	}
	p.UnmatchedExts = unmatchedExts
	p.UnmatchedLibs = unmatchedLibs
	return nil
}

// matchExtensionLibs matches extensions with their shared libraries
func (p *PostgresInstallation) matchExtensionLibs() error {
	// logrus.Debugf("matching extension libs for PostgreSQL %d.%d", p.MajorVersion, p.MinorVersion)
	for _, ext := range p.Extensions {
		if ext.Library != nil {
			continue
		}
		if lib, exists := p.LibMap[ext.Name]; exists {
			ext.Library = lib
			lib.Ext = ext
			if ext.Mtime.IsZero() {
				ext.Mtime = lib.Mtime
			}
			continue
		}
		for _, lib := range p.SharedLibs {
			if lib.ExtName == ext.Name {
				ext.Library = lib
				lib.Ext = ext
				if ext.Mtime.IsZero() {
					ext.Mtime = lib.Mtime
				}
				continue
			}
		}
	}

	return nil
}

// scanSharedLibs scans shared library files
func (p *PostgresInstallation) scanExtensions() error {
	//logrus.Debugf("scanning extension libs for PostgreSQL %d.%d", p.MajorVersion, p.MinorVersion)
	extensionsPath := filepath.Join(p.ExtPath)
	entries, err := os.ReadDir(extensionsPath)
	if err != nil {
		return fmt.Errorf("failed to read extensions directory: %v", err)
	}
	var extensions []*ExtensionCtrl
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".control") {
			extName := strings.TrimSuffix(entry.Name(), ".control")
			if badCaseExtensions[extName] {
				continue
			}
			ext := &ExtensionCtrl{
				Name: extName,
			}
			if err := p.parseControlFile(ext); err != nil {
				continue
			}
			extensions = append(extensions, ext)
		}
	}
	// add sqlLessExtensions (which does not have control file)
	for name, description := range sqlLessExtensions {
		// if found shared lib, add it to extension
		if lib, exists := p.LibMap[name]; exists {
			ext := &ExtensionCtrl{
				Name:        name,
				Description: description,
				Library:     lib,
			}
			extensions = append(extensions, ext)
			lib.Ext = ext
		}
	}

	p.Extensions = extensions
	p.ExtMap = make(map[string]*ExtensionCtrl)
	for _, ext := range extensions {
		p.ExtMap[ext.Name] = ext
	}
	return nil
}

// scanSharedLibs scans shared library files
func (p *PostgresInstallation) scanSharedLibs() error {
	entries, err := os.ReadDir(p.LibPath)
	if err != nil {
		return err
	}
	shareLibs := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".so") || strings.HasSuffix(entry.Name(), ".dylib")) {
			libName := strings.TrimSuffix(entry.Name(), ".so")
			libName = strings.TrimSuffix(libName, ".dylib")
			shareLibs[libName] = true
		}
	}
	return nil
}

// parseControlFile parses the control file of an extension
func (p *PostgresInstallation) parseControlFile(ext *ExtensionCtrl) error {
	controlPath := filepath.Join(p.ExtPath, ext.Name+".control")
	file, err := os.Open(controlPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get the creation time of the control file
	if info, err := file.Stat(); err == nil {
		ext.Mtime = info.ModTime()
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

// detectActiveInstall detects the active PostgreSQL installation
func detectActiveInstall() (*PostgresInstallation, error) {
	pgConfig, err := exec.LookPath("pg_config")
	if err != nil {
		return nil, fmt.Errorf("pg_config not found in PATH: %v", err)
	}
	install := &PostgresInstallation{}
	install.PgConfig = pgConfig

	// get the absolute path of pg config and detect it
	absPgConfigPath, err := filepath.Abs(pgConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of pg_config: %v", err)
	}
	install.PgConfig = absPgConfigPath

	if err := install.detectFromConfig(absPgConfigPath); err != nil {
		return nil, fmt.Errorf("failed to detect PostgreSQL from %s: %v", absPgConfigPath, err)
	}
	return install, nil
}

func PrintInstalledPostgres() string {
	if Installs == nil {
		return ""
	}
	var pgVerList []int
	for v := range Installs {
		pgVerList = append(pgVerList, v)
	}

	// sort in reverse
	sort.Sort(sort.Reverse(sort.IntSlice(pgVerList)))
	if len(pgVerList) == 0 {
		return "no installation found"
	}
	if len(pgVerList) == 1 {
		return fmt.Sprintf("%d (active)", pgVerList[0])
	}
	var pgVerStrList []string
	for _, v := range pgVerList {
		if Active != nil && v == Active.MajorVersion {
			pgVerStrList = append(pgVerStrList, fmt.Sprintf("%d (active)", v))
		} else {
			pgVerStrList = append(pgVerStrList, fmt.Sprintf("%d", v))
		}
	}

	return strings.Join(pgVerStrList, ", ")
}
