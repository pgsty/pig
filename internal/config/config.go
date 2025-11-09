package config

import (
	"bufio"
	"errors"
	"fmt"
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
	PigstyIO       = "https://pigsty.io"
	PigstyCC       = "https://pigsty.cc"
	PgstyCom       = "https://pgsty.com"
	RepoPigstyIO   = "https://repo.pigsty.io"
	RepoPigstyCC   = "https://repo.pigsty.cc"
	PigstyGPGCheck = false
	DistroEL       = "rpm"
	DistroDEB      = "deb"
	DistroMAC      = "brew"
)

// Build information. Populated at build-time via ldflags.
// BuildDate format follows RFC3339: YYYY-MM-DDTHH:MM:SSZ (e.g., 2025-01-10T10:20:00Z)
// This matches the format used in Makefile: date -u +'%Y-%m-%dT%H:%M:%SZ'
var (
	PigVersion    = "0.7.1"
	PigstyVersion = "3.6.1"
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
		os.MkdirAll(ConfigDir, 0750)
	}
	// touch config file if not exists
	if _, err := os.Stat(ConfigFile); os.IsNotExist(err) {
		os.Create(ConfigFile)
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
	PigstyConfig = findInventoryPath(inventory)

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
		pigstyHomePath = pigstyHome
		if validatePigstyHome(pigstyHomePath) {
			logrus.Debugf("pigsty home from cli arg: %s", pigstyHomePath)
			return pigstyHomePath
		}
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

func findInventoryPath(inventory string) string {
	var inventoryPaths, inventorySources []string

	if inventory != "" {
		// if relative path, convert to absolute path with current working directory
		if !filepath.IsAbs(inventory) {
			cwd, err := os.Getwd()
			if err != nil {
				cwd = "."
			}
			inventory = filepath.Join(cwd, inventory)
		}
		inventoryPaths = append(inventoryPaths, inventory)
		inventorySources = append(inventorySources, "cli arg")
	}

	if configInventory := viper.GetString("inventory"); configInventory != "" {
		inventoryPaths = append(inventoryPaths, configInventory)
		inventorySources = append(inventorySources, "config/env")
	}

	defaultPath := filepath.Join(HomeDir, "pigsty", "pigsty.yml")
	inventoryPaths = append(inventoryPaths, defaultPath)
	inventorySources = append(inventorySources, "default")

	for i, path := range inventoryPaths {
		if _, err := os.Stat(path); err == nil {
			logrus.Debugf("inventory found (%s): %s", inventorySources[i], path)
			return path
		}
	}
	logrus.Debugf("inventory not found")
	return ""
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

// DetectEnvironment detects the OS and sets the global variables
func DetectEnvironment() {
	OSArch = runtime.GOARCH
	NodeHostname, _ = os.Hostname()
	NodeCPUCount = runtime.NumCPU()

	// Priority 1: Check if we're root by UID (most reliable in Docker)
	if os.Geteuid() == 0 {
		CurrentUser = "root"
		logrus.Debugf("detected root user by UID")
	} else if user, err := user.Current(); err == nil {
		// Priority 2: Use system user detection
		CurrentUser = user.Username
		logrus.Debugf("detected user: %s", CurrentUser)
	} else {
		// Priority 3: Fallback to environment variable
		logrus.Debugf("could not determine current user: %v", err)
		if envUser := os.Getenv("USER"); envUser != "" {
			CurrentUser = envUser
			logrus.Debugf("using USER env variable: %s", CurrentUser)
		} else {
			CurrentUser = "unknown"
			logrus.Warnf("could not determine current user, using 'unknown'")
		}
	}
	if runtime.GOOS != "linux" {
		if runtime.GOOS == "darwin" {
			OSVendor = "macos"
			OSType = DistroMAC
			osVersion, err := exec.Command("uname", "-r").Output()
			if err != nil {
				logrus.Debugf("Failed to get os version from uname: %s", err)
				return
			} else {
				OSVersionFull = strings.TrimSpace(string(osVersion))
			}
			if OSVersionFull != "" {
				OSVersion = strings.Split(OSVersionFull, ".")[0]
				OSMajor, _ = strconv.Atoi(OSVersion)
				OSCode = fmt.Sprintf("a%s", OSVersion)
				OSVersionCode = OSCode
			}
			return
		}
		logrus.Debugf("Running on non-Linux platform: %s", runtime.GOOS)
		return
	}

	// First determine OS type by checking package manager
	if _, err := os.Stat("/usr/bin/rpm"); err == nil {
		OSType = DistroEL
	}
	if _, err := os.Stat("/usr/bin/dpkg"); err == nil {
		OSType = DistroDEB
	}

	// Try to read OS release info
	f, err := os.Open("/etc/os-release")
	if err != nil {
		logrus.Debugf("could not read /etc/os-release: %s", err)
		return
	}
	defer f.Close()

	var versionID string
	scanner := bufio.NewScanner(f)
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
			OSVendor = val
		case "VERSION_ID":
			versionID = val
			OSVersionFull = val
		case "VERSION_CODENAME":
			OSVersionCode = val
		}
	}

	// Extract major version
	if versionID != "" {
		OSVersion = strings.Split(versionID, ".")[0]
	}

	// Determine OS code based on distribution and package type
	if OSType == DistroEL {
		OSCode = "el" + OSVersion
		OSVersionCode = OSCode
	}
	if OSType == DistroDEB {
		if OSVendor == "ubuntu" {
			OSCode = "u" + OSVersion
		} else {
			OSCode = "d" + OSVersion
		}
	}
	logrus.Debugf("Detected OS: code=%s arch=%s type=%s vendor=%s version=%s %s",
		OSCode, OSArch, OSType, OSVendor, OSVersion, OSVersionCode)
}
