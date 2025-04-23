package do

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

// RunPlaybook runs the ansible playbook with the given inventory and args
func RunPlaybook(inventory string, args []string) error {
	logrus.Debugf("run playbook with inventory: %s, args: %v", inventory, strings.Join(args, " "))

	// run precheck
	if err := runPlaybookPrecheck(inventory); err != nil {
		return err
	}
	command := []string{"ansible-playbook"}
	if inventory != "" {
		command = append(command, "-i", inventory)
	}
	command = append(command, args...)
	if err := os.Chdir(config.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", config.PigstyHome, err)
	}

	logrus.Infof("run: %s", strings.Join(command, " "))
	return utils.Command(command)
}

// RunAnsible will run ansible command on pigsty home dir
func RunAnsible(inventory string, args []string) error {
	logrus.Debugf("run ansible command: %s", strings.Join(args, " "))

	// run precheck
	if err := runPlaybookPrecheck(inventory); err != nil {
		return err
	}

	command := []string{"ansible"}
	if inventory != "" {
		command = append(command, "-i", inventory)
	}
	command = append(command, args...)
	if err := os.Chdir(config.PigstyHome); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", config.PigstyHome, err)
	}
	logrus.Infof("run: %s", strings.Join(command, " "))
	return utils.Command(command)
}

func runPlaybookPrecheck(inventory string) error {

	if config.PigstyHome == "" {
		logrus.Errorf("PIGSTY_HOME not found, did you install pigsty?")
		return errors.New("pigsty home not found")
	}

	// check ansible playbook command exists
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		return fmt.Errorf("ansible-playbook command not found: %w", err)
	}

	// check pigsty home exists
	if _, err := os.Stat(config.PigstyHome); os.IsNotExist(err) {
		return fmt.Errorf("pigsty home %s not found: %w", config.PigstyHome, err)
	}

	if inventory != "" {
		// check the abs / relative path exists
		if _, err := os.Stat(inventory); os.IsNotExist(err) {
			return fmt.Errorf("the given inventory %s not found: %w", inventory, err)
		}
	} else {
		if config.PigstyConfig == "" {
			return errors.New("pigsty config inventory not found, use -i to specify the inventory")
		}
	}
	return nil
}
