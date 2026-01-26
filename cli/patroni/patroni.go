package patroni

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// ConfigPaths contains the possible paths to patroni config file (in priority order)
var ConfigPaths = []string{
	"/infra/conf/patronictl.yml", // admin node conf
	"/etc/patroni/patroni.yml",   // pgsql node conf
}

// findConfig returns the path to the first readable patroni config file
func findConfig() string {
	for _, path := range ConfigPaths {
		if file, err := os.Open(path); err == nil {
			file.Close()
			return path
		}
	}
	return ""
}

// buildBaseArgs builds common patronictl arguments (-c, -d, -k)
func buildBaseArgs(configFile, dcsURL string, insecure bool) []string {
	var args []string

	// Use provided config or auto-detect
	config := configFile
	if config == "" {
		config = findConfig()
	}
	if config != "" {
		args = append(args, "-c", config)
	}

	if dcsURL != "" {
		args = append(args, "-d", dcsURL)
	}
	if insecure {
		args = append(args, "-k")
	}
	return args
}

// runPatronictl executes patronictl with given arguments
func runPatronictl(args []string) error {
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return fmt.Errorf("patronictl not found in PATH, please install patroni first")
	}

	logrus.Debugf("patronictl %s", strings.Join(args, " "))
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// List runs patronictl list with -e -t flags
func List(configFile, dcsURL string, insecure bool, cluster string, watch bool, interval string) error {
	args := buildBaseArgs(configFile, dcsURL, insecure)
	args = append(args, "list", "-e", "-t")

	if watch {
		args = append(args, "-W")
	}
	if interval != "" {
		args = append(args, "-w", interval)
	}
	if cluster != "" {
		args = append(args, cluster)
	}

	return runPatronictl(args)
}

// Config shows or edits cluster configuration
func Config(configFile, dcsURL string, insecure bool, kvPairs []string) error {
	args := buildBaseArgs(configFile, dcsURL, insecure)

	if len(kvPairs) == 0 {
		// No key=value pairs, show config
		args = append(args, "show-config")
		return runPatronictl(args)
	}

	// Edit config with key=value pairs
	args = append(args, "edit-config", "--force")
	for _, kv := range kvPairs {
		args = append(args, "-s", kv)
	}
	return runPatronictl(args)
}

// Systemctl runs systemctl command for patroni service
func Systemctl(action string) error {
	logrus.Debugf("systemctl %s patroni", action)
	cmd := exec.Command("sudo", "systemctl", action, "patroni")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Log views patroni logs using journalctl
func Log(follow bool, lines string) error {
	args := []string{"-u", "patroni"}
	if follow {
		args = append(args, "-f")
	}
	if lines != "" {
		args = append(args, "-n", lines)
	}

	logrus.Debugf("journalctl %s", strings.Join(args, " "))
	cmd := exec.Command("journalctl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
