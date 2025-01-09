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
	OSVersion     string // 7/8/9/11/12/20/22/24
	OSMajor       int    // 7/8/9/11/12/20/22/24 (int format)
	OSVersionFull string // 9.3 / 22.04 / 12 from VERSION_ID
	OSVersionCode string // OS full version string
	CurrentUser   string // current user
	NodeHostname  string // hostname from /etc/hostname
	NodeCPUCount  int    // cpu count from /proc/cpuinfo
)

const (
	PigVersion     = "0.1.1"
	PigstyVersion  = "3.2.0"
	PigstyGPGCheck = false
	DistroEL       = "rpm"
	DistroDEB      = "deb"
	DistroMAC      = "brew"
)

func InitConfig(inventory string) {
	DetectEnvironment()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logrus.Debug("Failed to get user home directory, trying user.Current()")
		if usr, err := user.Current(); err == nil {
			homeDir = "/home/" + usr.Username
		} else {
			logrus.Fatalf("Failed to get user home directory via username, abort: %v", err)
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
			logrus.WithError(err).Debugf("Config file not found, will rely on environment variables and defaults")
		} else {
			logrus.WithError(err).Debugf("Error reading config file from %s", ConfigFile)
		}
	} else {
		logrus.Debugf("Load config from %s", ConfigFile)
	}

	// load config file
	cfgPath := viper.GetString("config")
	if cfgPath != "" {
		InitConfigFile(cfgPath)
	}

	// setup inventory & pigsty home
	if inventory != "" {
		PigstyConfig = inventory
		PigstyHome = filepath.Dir(inventory)
		logrus.Debugf("inventory = %s, home = %s, from cli arg", PigstyConfig, PigstyHome)
	} else {
		if inventory = viper.GetString("inventory"); inventory != "" {
			PigstyConfig = inventory
			PigstyHome = filepath.Dir(inventory)
			logrus.Debugf("inventory = %s, home = %s, from config/env", PigstyConfig, PigstyHome)
		} else {
			PigstyConfig = filepath.Join(HomeDir, "pigsty", "pigsty.yml")
			PigstyHome = filepath.Join(HomeDir, "pigsty")
			logrus.Debugf("inventory = %s, home = %s, from default", PigstyConfig, PigstyHome)
		}
	}

	// setup license if provided
	lic := viper.GetString("license")
	if lic != "" {
		license.InitLicense(lic)
	}
}

// InitConfigFile will init the config file with provided path
func InitConfigFile(cfgPath string) {
	viper.SetConfigType("yml")
	viper.SetDefault("license", "")
	viper.SetDefault("region", "default")
	viper.SetDefault("home", "~/pigsty")

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

	if cfgPath == "" {
		pigstyHome := os.Getenv("PIGSTY_HOME")
		if pigstyHome == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				logrus.Debug("Failed to get user home directory")
				pigstyHome = "${HOME}/pigsty"
			} else {
				pigstyHome = filepath.Join(homeDir, "pigsty")
				logrus.Debugf("config file is infer from ENV: PIGSTY_HOME=%s", pigstyHome)
			}
		}
		cfgPath = filepath.Join(pigstyHome, "pigsty.yml")
		cfgSource = "HOME"
	}

	PigstyConfig = cfgPath
	PigstyHome = filepath.Base(cfgPath)
	viper.SetConfigFile(cfgPath)

	viper.SetEnvPrefix("PIGSTY")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			logrus.WithError(err).Debugf("Config file not found, will rely on environment variables and defaults")
		} else {
			logrus.WithError(err).Debugf("Error reading config file from %s", cfgSource)
		}
	} else {
		logrus.Debugf("Load config from %s: %s", cfgSource, cfgPath)
	}
}

// DetectEnvironment detects the OS and sets the global variables
func DetectEnvironment() {
	OSArch = runtime.GOARCH
	NodeHostname, _ = os.Hostname()
	NodeCPUCount = runtime.NumCPU()
	if user, err := user.Current(); err == nil {
		CurrentUser = user.Username
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
