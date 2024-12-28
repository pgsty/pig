package repo

import (
	"fmt"
	"os"
	"os/exec"
)

// Boot will bootstrap a local repo from offline package
func Boot(targetDir, offlinePkg string) error {
	if targetDir == "" {
		targetDir = "/www/pigsty"
	}
	if offlinePkg == "" {
		offlinePkg = "/tmp/pkg.tgz"
	}

	// check if offline package exists
	if _, err := os.Stat(offlinePkg); os.IsNotExist(err) {
		return fmt.Errorf("offline package not found: %s", offlinePkg)
	}

	// ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %s", err)
	}

	// extract package using tar
	cmd := exec.Command("tar", "xzf", offlinePkg, "-C", targetDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract package: %s\nOutput: %s", err, output)
	}

	fmt.Printf("Successfully extracted %s to %s\n", offlinePkg, targetDir)
	return nil
}
