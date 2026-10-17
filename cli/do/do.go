package do

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// RunPlaybook runs the ansible playbook with the given inventory and args
func RunPlaybook(inventory string, args []string) error {
	logrus.Debugf("run playbook with inventory: %s, args: %v", inventory, strings.Join(args, " "))
	return runAnsibleTool("ansible-playbook", inventory, args)
}

// RunAnsible will run ansible command on pigsty home dir
func RunAnsible(inventory string, args []string) error {
	logrus.Debugf("run ansible command: %s", strings.Join(args, " "))
	return runAnsibleTool("ansible", inventory, args)
}

func runAnsibleTool(binary, inventory string, args []string) error {
	if config.PigstyHome == "" {
		logrus.Errorf("PIGSTY_HOME not found, did you install pigsty?")
		return errors.New("pigsty home not found")
	}

	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s command not found: %w", binary, err)
	}

	homeInfo, err := os.Stat(config.PigstyHome)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("pigsty home %s not found: %w", config.PigstyHome, err)
		}
		return fmt.Errorf("failed to inspect pigsty home %s: %w", config.PigstyHome, err)
	}
	if !homeInfo.IsDir() {
		return fmt.Errorf("pigsty home %s is not a directory", config.PigstyHome)
	}

	if inventory != "" {
		inventoryPath := inventory
		if !filepath.IsAbs(inventoryPath) {
			inventoryPath = filepath.Join(config.PigstyHome, inventoryPath)
		}
		if _, err := os.Stat(inventoryPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("the given inventory %s not found: %w", inventory, err)
			}
			return fmt.Errorf("failed to inspect inventory %s: %w", inventory, err)
		}
	} else {
		if config.PigstyConfig == "" {
			return errors.New("pigsty config inventory not found, use -i to specify the inventory")
		}
		if _, err := os.Stat(config.PigstyConfig); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("pigsty config inventory not found: %w", err)
			}
			return fmt.Errorf("failed to inspect inventory %s: %w", config.PigstyConfig, err)
		}
		inventory = config.PigstyConfig
	}

	command := []string{binary}
	if inventory != "" {
		command = append(command, "-i", inventory)
	}
	command = append(command, args...)
	logrus.Infof("run: %s", strings.Join(command, " "))
	return utils.CommandInDir(command, config.PigstyHome)
}
