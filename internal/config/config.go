package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"pig/cli/license"
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	ConfigDir     string
	ConfigFile    string
	HomeDir       string
	PigstyHome    string
	PigstyConfig  string
	OSArch        string // CPU architecture (amd64, arm64)
	OSCode        string // Distribution version (el8, el9, d12, u22)
	OSType        string // rpm / deb
	OSVendor      string // rocky/debian/ubuntu from ID
	OSVersion     string // 7/8/9/10/11/12/13/20/22/24
	OSMajor       int    // 7/8/9/10/11/12/13/20/22/24 (int format)
	OSVersionFull string // 9.6 / 22.04 / 12 from VERSION_ID
	OSVersionCode string // OS full version string
	CurrentUser   string // current user
	NodeHostname  string // hostname from /etc/hostname
	NodeCPUCount  int    // cpu count from /proc/cpuinfo
)

const (
	PigstyIO     = "https://pigsty.io"
	PigstyCC     = "https://pigsty.cc"
	RepoPigstyIO = "https://repo.pigsty.io"
	RepoPigstyCC = "https://repo.pigsty.cc"
	// PigstyGPGCheck intentionally defaults to false.
	// This project supports offline/mirror environments where enforcing signatures
	// and HTTPS-only repos is operationally incompatible by default.
	PigstyGPGCheck = false
	DistroEL       = "rpm"
	DistroDEB      = "deb"
	DistroMAC      = "brew"
)

// Output format constants
const (
	OUTPUT_TEXT        = "text"
	OUTPUT_YAML        = "yaml"
	OUTPUT_JSON        = "json"
	OUTPUT_JSON_PRETTY = "json-pretty"
)

// ValidOutputFormats contains all valid output format values for CLI flag
var ValidOutputFormats = []string{OUTPUT_TEXT, OUTPUT_YAML, OUTPUT_JSON, OUTPUT_JSON_PRETTY}

// OutputFormat is the global output format setting (default: text)
var OutputFormat = OUTPUT_TEXT

// IsStructuredOutput returns true if the current output format is structured (YAML/JSON)
// rather than plain text. Useful for suppressing progress output in structured mode.
func IsStructuredOutput() bool {
	return OutputFormat == OUTPUT_YAML || OutputFormat == OUTPUT_JSON || OutputFormat == OUTPUT_JSON_PRETTY
}

// Build information. Populated at build-time via ldflags.
// BuildDate format follows RFC3339: YYYY-MM-DDTHH:MM:SSZ (e.g., 2025-01-10T10:20:00Z)
// This matches the format used in Makefile: date -u +'%Y-%m-%dT%H:%M:%SZ'
var (
	PigVersion    = "1.6.0"
	PigstyVersion = "4.4.0"
	Branch        = "main"        // Will be set during release build
	Revision      = "HEAD"        // Will be set to commit hash during release build
	BuildDate     = "development" // Will be set to RFC3339 format during release build
	GoVersion     = runtime.Version()
	GOOS          = runtime.GOOS
	GOARCH        = runtime.GOARCH
)

// InitConfig initializes the configuration, if inventory and pigstyHome is given as cli args,
// if not, it will use the environment variables / config file / default values
func InitConfig(inventory, pigstyHome string) {
	DetectEnvironment()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logrus.Debugf("failed to get user home directory, trying user.Current()")
		if usr, err := user.Current(); err == nil {
			homeDir = "/home/" + usr.Username
		} else if os.Getuid() == 0 {
			homeDir = "/root"
		} else {
			logrus.Fatalf("failed to determine user home directory: %v", err)
		}
	}

	// set home dir, config dir, config file
	HomeDir = homeDir
	ConfigDir = filepath.Join(HomeDir, ".pig")
	ConfigFile = filepath.Join(ConfigDir, "config.yml")
	// create that directory if not exists
	if _, err := os.Stat(ConfigDir); os.IsNotExist(err) {
		if err := os.MkdirAll(ConfigDir, 0750); err != nil {
			logrus.Warnf("failed to create config directory %s: %v", ConfigDir, err)
		}
	}
	// touch config file if not exists
	if _, err := os.Stat(ConfigFile); os.IsNotExist(err) {
		f, err := os.Create(ConfigFile)
		if err != nil {
			logrus.Warnf("failed to create config file %s: %v", ConfigFile, err)
		} else {
			_ = f.Close()
		}
	}

	// set config defaults
	viper.SetConfigType("yml")
	viper.SetDefault("license", "")
	viper.SetDefault("inventory", "")
	viper.SetConfigFile(ConfigFile)
	viper.SetEnvPrefix("PIGSTY")
	viper.AutomaticEnv()

	// load config file
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			logrus.Debugf("config file not found, using environment variables and defaults")
		} else {
			logrus.Debugf("failed to read config file %s: %v", ConfigFile, err)
		}
	} else {
		logrus.Debugf("config loaded: %s", ConfigFile)
	}

	// load specified config file if provided
	cfgPath := viper.GetString("config")
	if cfgPath != "" {
		InitConfigFile(cfgPath)
	}

	// setup pigsty home
	PigstyHome = findPigstyHome(pigstyHome)

	// setup inventory
	PigstyConfig = findInventoryPath(inventory, PigstyHome)

	// fill missing pigsty.yml if pigsty home is set
	if PigstyHome != "" && PigstyConfig == "" {
		candidatePath := filepath.Join(PigstyHome, "pigsty.yml")
		if _, err := os.Stat(candidatePath); err == nil {
			PigstyConfig = candidatePath
		}
	}

	// setup license with HomeDir
	license.HomeDir = HomeDir

	// setup license if provided
	lic := viper.GetString("license")
	if lic != "" {
		license.InitLicense(lic)
	}
}

func findPigstyHome(pigstyHome string) string {
	var pigstyHomePath string
	if pigstyHome != "" {
		if !filepath.IsAbs(pigstyHome) {
			cwd, err := os.Getwd()
			if err != nil {
				logrus.Warnf("failed to get current working directory: %v", err)
				cwd = "."
			}
			pigstyHome = filepath.Join(cwd, pigstyHome)
		}
		pigstyHomePath = filepath.Clean(pigstyHome)
		if !validatePigstyHome(pigstyHomePath) {
			logrus.Debugf("pigsty home from cli arg does not contain ansible.cfg: %s", pigstyHomePath)
		}
		logrus.Debugf("pigsty home from cli arg: %s", pigstyHomePath)
		return pigstyHomePath
	}

	// if pigstyHomePath is not set or not valid, use home from config
	pigstyHomePath = viper.GetString("home")
	if validatePigstyHome(pigstyHomePath) {
		logrus.Debugf("pigsty home from config: %s", pigstyHomePath)
		return pigstyHomePath
	}

	pigstyHomePath = filepath.Join(HomeDir, "pigsty")
	if validatePigstyHome(pigstyHomePath) {
		logrus.Debugf("pigsty home from default: %s", pigstyHomePath)
		return pigstyHomePath
	}

	logrus.Debugf("pigsty home not found")
	return ""
}

func validatePigstyHome(pigstyHomePath string) bool {
	if pigstyHomePath == "" {
		return false
	}

	f, err := os.Open(filepath.Join(pigstyHomePath, "ansible.cfg"))
	if err == nil {
		defer f.Close()
		return true
	}
	return false
}

func findInventoryPath(inventory, pigstyHome string) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	choose := func(value, source, base string) string {
		path := resolveInventoryCandidate(value, base)
		if path != "" {
			logrus.Debugf("inventory selected (%s): %s", source, path)
		}
		return path
	}

	// An invocation-specific source is authoritative even when it does not yet
	// exist. Callers must report that concrete source instead of silently
	// falling through to an unrelated Inventory.
	if path := choose(inventory, "cli arg", cwd); path != "" {
		return path
	}
	if path := choose(os.Getenv("PIGSTY_INVENTORY"), "environment", cwd); path != "" {
		return path
	}
	if path := choose(viper.GetString("inventory"), "config", cwd); path != "" {
		return path
	}

	if pigstyHome != "" {
		if value, err := readAnsibleInventoryDirective(filepath.Join(pigstyHome, "ansible.cfg")); err == nil {
			if path := choose(value, "ansible.cfg", pigstyHome); path != "" {
				return path
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			logrus.Debugf("failed to read ansible.cfg inventory directive: %v", err)
		}
		return choose("pigsty.yml", "pigsty home default", pigstyHome)
	}

	logrus.Debugf("inventory source is not configured")
	return ""
}

func resolveInventoryCandidate(value, base string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if value == "~" {
		value = HomeDir
	} else if strings.HasPrefix(value, "~/") {
		value = filepath.Join(HomeDir, strings.TrimPrefix(value, "~/"))
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Clean(value)
}

// readAnsibleInventoryDirective deliberately reads only the inventory setting
// in the [defaults] section. It is not a general INI parser and does not infer
// an Inventory source from database reachability or adjacent files.
func readAnsibleInventoryDirective(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	inDefaults := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDefaults = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "defaults")
			continue
		}
		if !inDefaults {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "inventory") {
			continue
		}
		return cleanAnsibleInventoryValue(value), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func cleanAnsibleInventoryValue(value string) string {
	value = strings.TrimSpace(value)
	for index := 0; index < len(value); index++ {
		if (value[index] == '#' || value[index] == ';') && index > 0 && (value[index-1] == ' ' || value[index-1] == '\t') {
			value = strings.TrimSpace(value[:index])
			break
		}
	}
	if len(value) >= 2 && (value[0] == '\'' && value[len(value)-1] == '\'' || value[0] == '"' && value[len(value)-1] == '"') {
		value = value[1 : len(value)-1]
	}
	return strings.TrimSpace(value)
}

// InitConfigFile will init the config file with provided path
func InitConfigFile(cfgPath string) {
	viper.SetConfigType("yml")
	viper.SetDefault("license", "")
	viper.SetDefault("region", "default")
	viper.SetDefault("home", filepath.Join(HomeDir, "pigsty"))

	var cfgSource string
	if cfgPath != "" {
		cfgSource = "CLI"
		logrus.Debugf("config file %s is given through CLI", cfgPath)
	} else {
		cfgPath = os.Getenv("PIGSTY_CONFIG")
		if cfgPath != "" {
			logrus.Debugf("config file %s is given through ENV", cfgPath)
			cfgSource = "ENV"
		}
	}
	if cfgPath != "" && filepath.Ext(cfgPath) != ".yml" {
		logrus.Errorf("Given config file '%s' does not have .yml extension, ignoring it", cfgPath)
		cfgPath = ""
	}

	PigstyConfig = cfgPath
	viper.SetConfigFile(cfgPath)
	viper.SetEnvPrefix("PIGSTY")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			logrus.Debugf("config file not found, using environment variables and defaults")
		} else {
			logrus.Debugf("failed to read config file from %s: %v", cfgSource, err)
		}
	} else {
		logrus.Debugf("config loaded from %s: %s", cfgSource, cfgPath)
	}
}

type osReleaseInfo struct {
	ID              string
	VersionID       string
	VersionCodename string
}

func parseOSRelease(r io.Reader) osReleaseInfo {
	var info osReleaseInfo

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := strings.Trim(parts[1], "\"")
		switch key {
		case "ID":
			info.ID = val
		case "VERSION_ID":
			info.VersionID = val
		case "VERSION_CODENAME":
			info.VersionCodename = val
		}
	}
	return info
}

func readOSRelease(path string) (osReleaseInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return osReleaseInfo{}, err
	}
	defer f.Close()
	return parseOSRelease(f), nil
}

func detectCurrentUser() {
	// Priority 1: Check if we're root by UID (most reliable in Docker).
	if os.Geteuid() == 0 {
		CurrentUser = "root"
		logrus.Debugf("detected root user by UID")
		return
	}

	// Priority 2: Use system user detection.
	if user, err := user.Current(); err == nil {
		CurrentUser = user.Username
		logrus.Debugf("detected user: %s", CurrentUser)
		return
	} else {
		// Priority 3: Fallback to environment variable.
		logrus.Debugf("could not determine current user: %v", err)
	}

	if envUser := os.Getenv("USER"); envUser != "" {
		CurrentUser = envUser
		logrus.Debugf("using USER env variable: %s", CurrentUser)
		return
	}

	CurrentUser = "unknown"
	logrus.Warnf("could not determine current user, using 'unknown'")
}

func detectDarwinEnvironment() bool {
	if runtime.GOOS != "darwin" {
		return false
	}

	OSVendor = "macos"
	OSType = DistroMAC

	osVersion, err := exec.Command("uname", "-r").Output()
	if err != nil {
		logrus.Debugf("Failed to get os version from uname: %s", err)
		return true
	}

	OSVersionFull = strings.TrimSpace(string(osVersion))
	if OSVersionFull == "" {
		return true
	}

	OSVersion = strings.Split(OSVersionFull, ".")[0]
	OSMajor, _ = strconv.Atoi(OSVersion)
	OSCode = fmt.Sprintf("a%s", OSVersion)
	OSVersionCode = OSCode
	return true
}

func inferLinuxPackageTypeFromVendor(vendor string) string {
	switch strings.ToLower(vendor) {
	case "ubuntu", "debian":
		return DistroDEB
	case "rhel", "redhat", "centos", "rocky", "almalinux", "ol", "oracle", "fedora", "anolis", "amzn", "amazon":
		return DistroEL
	default:
		return ""
	}
}

func detectLinuxPackageManager(vendor string) {
	OSType = ""

	// Prefer distro vendor from /etc/os-release. This is more reliable than
	// checking whether helper binaries are present on mixed environments.
	if inferred := inferLinuxPackageTypeFromVendor(vendor); inferred != "" {
		OSType = inferred
		return
	}

	// Fallback to distro marker files.
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		OSType = DistroDEB
		return
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		OSType = DistroEL
		return
	}

	// Last fallback: package manager binaries.
	if _, err := os.Stat("/usr/bin/dpkg"); err == nil {
		OSType = DistroDEB
		return
	}
	if _, err := os.Stat("/usr/bin/rpm"); err == nil {
		OSType = DistroEL
	}
}

func applyLinuxReleaseInfo(info osReleaseInfo) {
	OSVendor = info.ID
	OSVersionFull = info.VersionID
	OSVersionCode = info.VersionCodename

	// Extract major version.
	if info.VersionID != "" {
		OSVersion = strings.Split(info.VersionID, ".")[0]
		OSMajor, _ = strconv.Atoi(OSVersion)
	}
}

func detectLinuxOSCode() {
	// Determine OS code based on distribution and package type.
	if OSType == DistroEL {
		OSCode = "el" + OSVersion
		OSVersionCode = OSCode
		return
	}

	if OSType == DistroDEB {
		if OSVendor == "ubuntu" {
			OSCode = "u" + OSVersion
		} else {
			OSCode = "d" + OSVersion
		}
	}
}

// DetectEnvironment detects the OS and sets the global variables
func DetectEnvironment() {
	OSArch = runtime.GOARCH
	NodeHostname, _ = os.Hostname()
	NodeCPUCount = runtime.NumCPU()

	detectCurrentUser()

	if runtime.GOOS != "linux" {
		if detectDarwinEnvironment() {
			return
		}
		logrus.Debugf("Running on non-Linux platform: %s", runtime.GOOS)
		return
	}

	info, err := readOSRelease("/etc/os-release")
	if err != nil {
		logrus.Debugf("could not read /etc/os-release: %s", err)
		detectLinuxPackageManager("")
		return
	}
	applyLinuxReleaseInfo(info)
	detectLinuxPackageManager(OSVendor)
	detectLinuxOSCode()

	logrus.Debugf("Detected OS: code=%s arch=%s type=%s vendor=%s version=%s %s major=%d full=%s",
		OSCode, OSArch, OSType, OSVendor, OSVersion, OSVersionCode, OSMajor, OSVersionFull)
}
