package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	PigVersion    = "0.0.1"
	PigstyVersion = "3.2.0"

	ConfigDir    string
	ConfigFile   string
	HomeDir      string
	PigstyHome   string
	PigstyConfig string

	OSArch        string // CPU architecture (amd64, arm64)
	OSCode        string // Distribution version (el8, el9, d12, u22)
	OSType        string // rpm / deb
	OSVendor      string // rocky/debian/ubuntu from ID
	OSVersion     string // 7/8/9/11/12/20/22/24
	OSVersionFull string // 9.3 / 22.04 / 12 from VERSION_ID
	OSVersionCode string // OS full version string
	NodeHostname  string // hostname from /etc/hostname
	NodeCPUCount  int    // cpu count from /proc/cpuinfo

)

func InitConfig(inventory string) {
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
	ConfigDir = filepath.Join(HomeDir, ".pigsty")
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

	cfgPath := viper.GetString("config")
	if cfgPath != "" {
		InitInventory(cfgPath)
	}

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
			logrus.Debugf("inventory = %s, home = %s, from defualt", PigstyConfig, PigstyHome)
		}
	}

	DetectOS()
}

func InitInventory(configFile string) {
	viper.SetConfigType("yml")
	viper.SetDefault("license", "")
	viper.SetDefault("region", "default")
	viper.SetDefault("home", "~/pigsty")

	var configSource string
	if configFile != "" {
		configSource = "CLI"
		logrus.Debugf("config file %s is given through CLI", configFile)
	} else {
		configFile = os.Getenv("PIGSTY_CONFIG")
		if configFile != "" {
			logrus.Debugf("config file %s is given through ENV", configFile)
			configSource = "ENV"
		}
	}
	if configFile != "" && filepath.Ext(configFile) != ".yml" {
		logrus.Errorf("Given config file '%s' does not have .yml extension, ignoring it", configFile)
		configFile = ""
	}

	if configFile == "" {
		// 优先级3: PIGSTY_HOME 环境变量或默认值
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
		configFile = filepath.Join(pigstyHome, "pigsty.yml")
		configSource = "HOME"
	}

	PigstyConfig = configFile
	PigstyHome = filepath.Base(configFile)
	viper.SetConfigFile(configFile)

	viper.SetEnvPrefix("PIGSTY")
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			logrus.WithError(err).Debugf("Config file not found, will rely on environment variables and defaults")
		} else {
			logrus.WithError(err).Debugf("Error reading config file from %s", configSource)
		}
	} else {
		logrus.Debugf("Load config from %s: %s", configSource, configFile)
	}
}

func DetectOS() {
	OSArch = runtime.GOARCH
	NodeHostname, _ = os.Hostname()
	NodeCPUCount = runtime.NumCPU()

	if runtime.GOOS != "linux" {
		if runtime.GOOS == "darwin" {
			OSVendor = "macos"
			OSType = "brew"
			osVersion, err := exec.Command("uname", "-r").Output()
			if err != nil {
				logrus.Debugf("Failed to get os version from uname: %s", err)
				return
			} else {
				OSVersionFull = strings.TrimSpace(string(osVersion))
			}
			if OSVersionFull != "" {
				OSVersion = strings.Split(OSVersionFull, ".")[0]
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
		OSType = "rpm"
	}
	if _, err := os.Stat("/usr/bin/dpkg"); err == nil {
		OSType = "deb"
	}

	// Try to read OS release info
	f, err := os.Open("/etc/os-release")
	if err != nil {
		logrus.Debugf("could not read /etc/os-release: %s", err)
		return
	}
	defer f.Close()

	var id, versionID string
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
			id = val
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
	if OSType == "rpm" {
		OSCode = "el" + OSVersion
		OSVersionCode = OSCode
	}
	if OSType == "deb" {
		if id == "ubuntu" {
			OSCode = "u" + OSVersion
		}
		OSCode = "d" + OSVersion
	}

	logrus.Debugf("Detected OS: code=%s arch=%s type=%s vendor=%s version=%s %s",
		OSCode, OSArch, OSType, OSVendor, OSVersion, OSVersionCode)
}