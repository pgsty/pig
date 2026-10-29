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
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	ConfigDir    string
	ConfigFile   string
	HomeDir      string
	PigstyHome   string
	PigstyConfig string
	// PigstyInventorySource records how PigstyConfig was selected without
	// retaining Inventory values or executing dynamic sources.
	PigstyInventorySource InventorySourceDescriptor
	OSArch                string // CPU architecture (amd64, arm64)
	OSCode                string // Distribution version (el8, el9, d12, u22)
	OSType                string // rpm / deb
	OSVendor              string // rocky/debian/ubuntu from ID
	OSVersion             string // 7/8/9/10/11/12/13/20/22/24
	OSMajor               int    // 7/8/9/10/11/12/13/20/22/24 (int format)
	OSVersionFull         string // 9.6 / 22.04 / 12 from VERSION_ID
	OSVersionCode         string // OS full version string
	CurrentUser           string // current user
	NodeHostname          string // hostname from /etc/hostname
	NodeCPUCount          int    // cpu count from /proc/cpuinfo
)

const (
	PigstyIO     = "https://pigsty.io"
	PigstyCC     = "https://pigsty.cc"
	RepoPigstyIO = "https://repo.pigsty.io"
	RepoPigstyCC = "https://repo.pigsty.cc"
	DistroEL     = "rpm"
	DistroDEB    = "deb"
	DistroMAC    = "brew"
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
	PigVersion    = "1.10.3"
	PigstyVersion = "4.5.0"
	Branch        = "main"        // Will be set during release build
	Revision      = "HEAD"        // Will be set to commit hash during release build
	BuildDate     = "development" // Will be set to RFC3339 format during release build
	GoVersion     = runtime.Version()
	GOOS          = runtime.GOOS
	GOARCH        = runtime.GOARCH
)

// InitOptions controls configuration initialization side effects.
type InitOptions struct {
	CreateConfig    bool
	SkipEnvironment bool
}

// InitConfigWithOptions initializes source selection. Read-only Inventory
// commands pass CreateConfig=false so discovery never creates ~/.pig or its
// config file.
func InitConfigWithOptions(inventory, pigstyHome string, options InitOptions) error {
	if !options.SkipEnvironment {
		DetectEnvironment()
	}
	homeDir, err := resolveUserHome()
	if err != nil {
		return err
	}

	// set home dir, config dir, config file
	HomeDir = homeDir
	ConfigDir = filepath.Join(HomeDir, ".pig")
	ConfigFile = filepath.Join(ConfigDir, "config.yml")
	if options.CreateConfig {
		if err := prepareConfigFile(); err != nil {
			return err
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
	if err := readDefaultConfig(); err != nil {
		return err
	}

	// load specified config file if provided
	cfgPath := viper.GetString("config")
	if cfgPath != "" {
		if err := InitConfigFile(cfgPath); err != nil {
			return err
		}
	}

	// setup pigsty home
	PigstyHome = findPigstyHome(pigstyHome)

	// setup inventory and retain its value-free provenance/status descriptor
	PigstyInventorySource = resolveInventorySource(inventory, PigstyHome)
	PigstyConfig = PigstyInventorySource.Path
	return nil
}

func resolveUserHome() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err == nil {
		return homeDir, nil
	}
	logrus.Debugf("failed to get user home directory, trying user.Current()")
	if usr, userErr := user.Current(); userErr == nil {
		if usr.HomeDir != "" {
			return usr.HomeDir, nil
		}
		return "/home/" + usr.Username, nil
	}
	if os.Getuid() == 0 {
		return "/root", nil
	}
	return "", fmt.Errorf("failed to determine user home directory: %w", err)
}

func prepareConfigFile() error {
	if info, err := os.Lstat(ConfigDir); os.IsNotExist(err) {
		if err := os.MkdirAll(ConfigDir, 0700); err != nil {
			return fmt.Errorf("failed to create config directory %s: %w", ConfigDir, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect config directory %s: %w", ConfigDir, err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("config directory is not a real directory: %s", ConfigDir)
	}
	if err := os.Chmod(ConfigDir, 0700); err != nil {
		return fmt.Errorf("failed to protect config directory %s: %w", ConfigDir, err)
	}
	if info, err := os.Lstat(ConfigFile); os.IsNotExist(err) {
		f, createErr := os.OpenFile(ConfigFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if createErr != nil {
			return fmt.Errorf("failed to create config file %s: %w", ConfigFile, createErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			return fmt.Errorf("failed to close config file %s: %w", ConfigFile, closeErr)
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect config file %s: %w", ConfigFile, err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("config file is not a regular file: %s", ConfigFile)
	}
	if err := os.Chmod(ConfigFile, 0600); err != nil {
		return fmt.Errorf("failed to protect config file %s: %w", ConfigFile, err)
	}
	return nil
}

func readDefaultConfig() error {
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) || errors.Is(err, os.ErrNotExist) {
			logrus.Debugf("config file not found, using environment variables and defaults")
			return nil
		}
		return fmt.Errorf("failed to read config file %s: %w", ConfigFile, err)
	}
	logrus.Debugf("config loaded: %s", ConfigFile)
	return nil
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

func expandHomePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "~" {
		return HomeDir
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(HomeDir, strings.TrimPrefix(value, "~/"))
	}
	return value
}

func resolveInventoryCandidate(value, base string) string {
	value = expandHomePath(value)
	if value == "" {
		return ""
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Clean(value)
}

// InitConfigFile will init the config file with provided path
func InitConfigFile(cfgPath string) error {
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
		return fmt.Errorf("given config file %q does not have .yml extension", cfgPath)
	}
	if cfgPath == "" {
		return nil
	}

	PigstyConfig = cfgPath
	viper.SetConfigFile(cfgPath)
	viper.SetEnvPrefix("PIGSTY")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("config file from %s not found: %s", cfgSource, cfgPath)
		}
		return fmt.Errorf("failed to read config file from %s (%s): %w", cfgSource, cfgPath, err)
	} else {
		logrus.Debugf("config loaded from %s: %s", cfgSource, cfgPath)
	}
	return nil
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
	OSCode = ""
	OSType = ""
	OSVendor = ""
	OSVersion = ""
	OSVersionFull = ""
	OSVersionCode = ""
	OSMajor = 0
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
