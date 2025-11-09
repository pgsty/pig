package ext

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
)

// PostgresInstall stores information about a PostgreSQL installation
type PostgresInstall struct {
	PgConfig     string                       // pg_config path
	PgConfigPath string                       // pg_config physical path
	PgConfigMap  map[string]string            // metadata from pg_config
	Version      string                       // the raw version string
	MajorVersion int                          // PostgreSQL major version
	MinorVersion int                          // PostgreSQL minor version
	BinPath      string                       // PostgreSQL binary path
	ExtPath      string                       // PostgreSQL extension path
	LibPath      string                       // PostgreSQL library path
	SharedLibs   map[string]bool              // shared libraries
	Extensions   []*ExtensionInstall          // installed extensions
	ExtensionMap map[string]*ExtensionInstall // extension map
}

var (
	Installs map[int]*PostgresInstall    // All installed PostgreSQL installations
	PathMap  map[string]*PostgresInstall // real pg_config path to pi map
	Active   *PostgresInstall            // The active PostgreSQL installation (in PATH)
	Postgres *PostgresInstall            // The designated PostgreSQL installation
	Inited   = false                     // Whether the PostgreSQL installation has been initialized
)

var (
	PostgresActiveMajorVersions = []int{18, 17, 16, 15, 14, 13}
	PostgresLatestMajorVersion  = 18
	PostgresElSearchPath        = []string{"/usr/pgsql-%s/bin/pg_config"}
	PostgresDEBSearchPath       = []string{"/usr/lib/postgresql/%s/bin/pg_config"}
	PostgresMACSearchPath       = []string{"/opt/homebrew/opt/postgresql@%s/bin/pg_config"}
)

// NewPostgresInstall hold the information of a PostgreSQL installation
func NewPostgresInstall(pgConfigPath string) (*PostgresInstall, error) {
	pi := &PostgresInstall{PgConfig: pgConfigPath}
	if err := pi.ScanMeta(); err != nil {
		return nil, fmt.Errorf("failed to detect PostgreSQL from %s: %v", pgConfigPath, err)
	}
	if err := pi.ScanExtensions(); err != nil {
		return pi, fmt.Errorf("failed to scan extensions for %s: %v", pgConfigPath, err)
	}
	return pi, nil
}

// ScanMeta retrieves installation information from the specified pg_config path
func (p *PostgresInstall) ScanMeta() error {
	// check pg_config exists and executable
	if info, err := os.Stat(p.PgConfig); err != nil {
		return fmt.Errorf("pg_config %s not found: %v", p.PgConfig, err)
	} else if info.Mode()&0111 == 0 {
		return fmt.Errorf("pg_config %s is not executable", p.PgConfig)
	}

	// read any symbolic link
	realPath, err := filepath.EvalSymlinks(p.PgConfig)
	if err != nil {
		logrus.Debugf("failed to resolve symbolic link %s: %v", p.PgConfig, err)
	} else {
		p.PgConfigPath = realPath
	}

	// run pg_config and parse the output
	cmd := exec.Command(p.PgConfig)
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
	p.PgConfigMap = configMap

	// parse the version
	if version, ok := configMap["VERSION"]; ok {
		p.Version = version
		p.MajorVersion, p.MinorVersion, err = utils.ParsePostgresVersion(p.Version)
		if err != nil {
			return fmt.Errorf("failed to parse PostgreSQL version: %v", err)
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

// String returns a string representation of the PostgreSQL installation
func (p *PostgresInstall) String() string {
	return fmt.Sprintf("PostgreSQL %d.%d: %s",
		p.MajorVersion,
		p.MinorVersion,
		p.PgConfig,
	)
}

// Summary prints a summary of the PostgreSQL installation
func (pg *PostgresInstall) Summary() {
	utils.PadKV("PostgreSQL "+fmt.Sprintf("%d.%d", pg.MajorVersion, pg.MinorVersion), pg.Version)
	utils.PadKV("Binary Path", pg.BinPath)
	utils.PadKV("Library Path", pg.LibPath)
	utils.PadKV("PgConfig Path", pg.PgConfig)
	utils.PadKV("Extension Path", pg.ExtPath)
}

// GetActivePgConfig returns the active pg_config path
func GetActivePgConfig() (string, error) {
	pgConfig, err := exec.LookPath("pg_config")
	if err != nil {
		return "", fmt.Errorf("pg_config not found in PATH: %v", err)
	}
	absPgConfigPath, err := filepath.Abs(pgConfig)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of pg_config: %v", err)
	}
	return absPgConfigPath, nil
}

// GetActivePostgresInstall returns the active PostgreSQL installation
func GetActivePostgresInstall() (*PostgresInstall, error) {
	pgConfigPath, err := GetActivePgConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get active pg_config: %v", err)
	}
	return NewPostgresInstall(pgConfigPath)
}

// DetectPostgres detects all installed PostgreSQL versions on the system
func DetectPostgres() error {
	allPostgres := make(map[int]*PostgresInstall)
	var searchPath []string

	// Determine the search path based on the OS type
	switch config.OSType {
	case config.DistroEL:
		searchPath = PostgresElSearchPath
	case config.DistroDEB:
		searchPath = PostgresDEBSearchPath
	case config.DistroMAC:
		searchPath = PostgresMACSearchPath
	default:
		return fmt.Errorf("unsupported OS type: %v", config.OSType)
	}

	// Get the active pg_config path
	activePhysicalPath, err := GetActivePgConfig()
	if err != nil {
		logrus.Debugf("failed to detect active PostgreSQL: %v", err)
		activePhysicalPath = ""
	} else {
		activePhysicalPath, err = filepath.EvalSymlinks(activePhysicalPath)
		if err != nil {
			logrus.Debugf("failed to resolve symbolic link %s: %v", activePhysicalPath, err)
			activePhysicalPath = ""
		}
	}

	// Iterate over possible PostgreSQL major versions
	for _, v := range PostgresActiveMajorVersions {
		if _, exists := Installs[v]; exists {
			continue
		}

		// Search for pg_config in the determined paths
		for _, pattern := range searchPath {
			verStr := strconv.Itoa(v)
			pgConfigPath := fmt.Sprintf(pattern, verStr)
			if _, err := os.Stat(pgConfigPath); err != nil {
				continue // not exists
			}

			logrus.Debugf("found pg_config %s", pgConfigPath)
			pi, err := NewPostgresInstall(pgConfigPath)
			if err != nil {
				logrus.Debugf("failed to detect PostgreSQL %d at %s: %v", v, pgConfigPath, err)
				continue
			}
			if activePhysicalPath != "" && pi.PgConfigPath == activePhysicalPath {
				logrus.Debugf("found active PostgreSQL %d at %s", pi.MajorVersion, pgConfigPath)
				Active = pi
			} else {
				logrus.Debugf("found PostgreSQL %d at %s", v, pgConfigPath)
			}
			allPostgres[pi.MajorVersion] = pi
		}
	}

	// If active is not found by iteration, try to find it by pg_config path
	if Active == nil && activePhysicalPath != "" {
		Active, err = NewPostgresInstall(activePhysicalPath)
		if err != nil {
			logrus.Debugf("failed to detect active PostgreSQL: %v", err)
		}
	}

	Installs = allPostgres
	PathMap = make(map[string]*PostgresInstall)
	for _, pi := range allPostgres {
		PathMap[pi.PgConfigPath] = pi
	}
	Inited = true
	return nil
}

// PostgresInstallSummary print the summary of PostgreSQL installation
func PostgresInstallSummary() {
	if !Inited {
		if err := DetectPostgres(); err != nil {
			logrus.Errorf("failed to detect PostgreSQL: %v", err)
			return
		}
	}
	// print installed PostgreSQL versions using tabwriter
	writer := new(tabwriter.Writer)
	writer.Init(os.Stdout, 0, 8, 2, ' ', 0)

	if len(Installs) > 0 {
		fmt.Fprintln(writer, "Installed:")
		for _, v := range Installs {
			if v == Active {
				fmt.Fprintf(writer, "* %s\t%s\n", v.Version, fmt.Sprintf("%-3d Extensions", len(v.Extensions)))
			}
		}
		for _, v := range Installs {
			if v != Active {
				fmt.Fprintf(writer, "- %s\t%s\n", v.Version, fmt.Sprintf("%-3d Extensions", len(v.Extensions)))
			}
		}
	} else {
		fmt.Fprintln(writer, "No PostgreSQL installation found")
	}

	// print active PostgreSQL detail using tabwriter
	if Active != nil {
		fmt.Fprintln(writer, "\nActive:")
		fmt.Fprintf(writer, "PG Version\t:  %s\n", Active.Version)
		fmt.Fprintf(writer, "Config Path\t:  %s\n", Active.PgConfig)
		fmt.Fprintf(writer, "Binary Path\t:  %s\n", Active.BinPath)
		fmt.Fprintf(writer, "Library Path\t:  %s\n", Active.LibPath)
		fmt.Fprintf(writer, "Extension Path\t:  %s\n", Active.ExtPath)
	} else {
		fmt.Fprintln(writer, "\nNo active PostgreSQL found in PATH:")
		// split the PATH and print each path
		paths := strings.Split(os.Getenv("PATH"), ":")
		for _, path := range paths {
			fmt.Fprintf(writer, "- %s\n", path)
		}
	}

	writer.Flush()
}

func GetPostgres(args ...string) (pi *PostgresInstall, err error) {
	// you can give at most 1 arg, could be a path, or version
	if len(args) == 0 {
		if Active != nil {
			Postgres = Active
			return Active, nil
		} else {
			return nil, fmt.Errorf("no args & no active postgres")
		}
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("too many arguments, only one path/version is allowed")
	}

	arg := args[0]
	if strings.HasSuffix(arg, "pg_config") {
		// read the path and check if it eix
		realPath, err := validatePgConfigPath(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to validate pg_config path %s: %v", arg, err)
		}
		// if it can be found in PathMap, return it
		if pi, ok := PathMap[realPath]; ok {
			Postgres = pi
			return pi, nil
		} else {
			Postgres, err = NewPostgresInstall(realPath)
			if err != nil {
				return nil, fmt.Errorf("failed to detect PostgreSQL from %s: %v", realPath, err)
			}
			return Postgres, nil
		}
	}

	// treat it as a version string
	major, _, err := utils.ParsePostgresVersion(arg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PostgreSQL version %s: %v", arg, err)
	}
	if pi, ok := Installs[major]; ok {
		Postgres = pi
		return pi, nil
	}
	return nil, fmt.Errorf("PostgreSQL %d not found", major)
}

func validatePgConfigPath(path string) (string, error) {
	if info, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("pg_config %s is not executable: %v", path, err)
	} else if info.Mode()&0111 == 0 {
		return "", fmt.Errorf("pg_config %s is not executable", path)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symbolic link %s: %v", path, err)
	}
	return realPath, nil
}
