package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/utils"
)

var (
	region   string
	pkgPath  string
	keepRepo bool
)

func Bootstrap() error {
	pigstyHome := os.Getenv("PIGSTY_HOME")
	if pigstyHome == "" {
		return fmt.Errorf("PIGSTY_HOME environment variable is not set")
	}

	bootstrapPath := filepath.Join(pigstyHome, "bootstrap")

	cmdArgs := []string{bootstrapPath}
	if region != "" {
		cmdArgs = append(cmdArgs, "-r", region)
	}
	if pkgPath != "" {
		cmdArgs = append(cmdArgs, "-p", pkgPath)
	}
	if keepRepo {
		cmdArgs = append(cmdArgs, "-k")
	}

	os.Chdir(pigstyHome)
	err := utils.ShellCommand(cmdArgs)
	if err != nil {
		return fmt.Errorf("bootstrap execution failed: %v", err)
	}

	return nil
}
