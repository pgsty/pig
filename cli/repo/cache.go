package repo

import (
	"fmt"
	"os"
	"os/exec"
)

// Cache will create an offline package from the specified directory
func Cache(sourceDir, outputPkg string) error {
	if sourceDir == "" {
		sourceDir = "/www/pigsty"
	}
	if outputPkg == "" {
		outputPkg = "/tmp/pkg.tgz"
	}

	// check if source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory not found: %s", sourceDir)
	}

	// ensure parent directory of output package exists
	if err := os.MkdirAll(outputPkg[:len(outputPkg)-len("/pkg.tgz")], 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %s", err)
	}

	// create tar archive
	cmd := exec.Command("tar", "czf", outputPkg, "-C", sourceDir, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create package: %s\nOutput: %s", err, output)
	}

	fmt.Printf("Successfully created offline package at %s from %s\n", outputPkg, sourceDir)
	return nil
}
