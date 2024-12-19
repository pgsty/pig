package config

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	ConfigDir  string
	ConfigFile string
	HomeDir    string

	PigstyHome   string
	PigstyConfig string
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
