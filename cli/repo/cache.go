package repo

import (
	"fmt"
	"os"
	"os/exec"
)

// Cache will create an offline package from the specified directory
func Cache(dirPath, pkgPath string) error {
	if dirPath == "" {
		dirPath = "/www/pigsty"
	}
	if pkgPath == "" {
		pkgPath = "/tmp/pkg.tgz"
	}

	// check if source directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return fmt.Errorf("source directory not found: %s", dirPath)
	}

	// ensure parent directory of output package exists
	if err := os.MkdirAll(pkgPath[:len(pkgPath)-len("/pkg.tgz")], 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %s", err)
	}

	// create tar archive
	cmd := exec.Command("tar", "czf", pkgPath, "-C", dirPath, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create package: %s\nOutput: %s", err, output)
	}

	fmt.Printf("Successfully created offline package at %s from %s\n", pkgPath, dirPath)
	return nil
}
