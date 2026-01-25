package patroni

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// ConfigPaths contains the possible paths to patroni config file
var ConfigPaths = []string{
	"/infra/conf/patronictl.yml", // on admin node
	"/etc/patroni/patroni.yml",   // on pgsql node
}

// FindConfig returns the path to the patroni config file
func FindConfig() (string, error) {
	for _, path := range ConfigPaths {
		// Check if file exists and is readable
		file, err := os.Open(path)
		if err == nil {
			file.Close()
			return path, nil
		}
	}
	return "", fmt.Errorf("error: patronictl config not found or not readable")
}

// Execute runs the patronictl command with the given arguments
func Execute(configPath string, dcsURL string, insecure bool, args []string) error {
	// Check if patronictl is available
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return fmt.Errorf("error: patronictl command not found in PATH, please install patroni first")
	} else {
		logrus.Debugf("patronictl found at %s", binPath)
	}

	patroniArgs := []string{}

	// Add config file if provided
	if configPath != "" {
		patroniArgs = append(patroniArgs, "-c", configPath)
	}

	// Add DCS URL if provided
	if dcsURL != "" {
		patroniArgs = append(patroniArgs, "-d", dcsURL)
	}

	// Add insecure flag if needed
	if insecure {
		patroniArgs = append(patroniArgs, "-k")
	}

	// Append all other arguments
	patroniArgs = append(patroniArgs, args...)

	// Create the command
	logrus.Debugf("patronictl %v", strings.Join(patroniArgs, " "))
	cmd := exec.Command(binPath, patroniArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute the command
	return cmd.Run()
}
