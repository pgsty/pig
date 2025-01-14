package ext

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"strconv"

	"github.com/sirupsen/logrus"
)

const (
	pgLinkPath    = "/usr/pgsql"
	pgProfilePath = "/etc/profile.d/pgsql.sh"
	minPGVersion  = 10
	maxPGVersion  = 30
)

var (
	unlinkKeywords = map[string]bool{"null": true, "none": true, "nil": true, "nop": true, "no": true}
	systemPaths    = map[string]bool{"/": true, "/usr": true, "/usr/local": true}
)

// UnlinkPostgres unlinks the current PostgreSQL:
// 1. Removes the /usr/pgsql symbolic link
// 2. Removes the /etc/profile.d/pgsql.sh file
func UnlinkPostgres() error {
	logrus.Info("unlinking postgres from environment")

	if err := RemoveSymlink(pgLinkPath); err != nil {
		return fmt.Errorf("failed to remove symlink %s: %w", pgLinkPath, err)
	}

	if err := utils.DelFile(pgProfilePath); err != nil {
		return fmt.Errorf("failed to remove profile file %s: %w", pgProfilePath, err)
	}

	logrus.Info("successfully unlinked postgres")
	return nil
}

// RemoveSymlink removes a symbolic link if it exists and is actually a symlink
func RemoveSymlink(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check symlink %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("path %s exists but is not a symbolic link", path)
	}
	return utils.DelFile(path)
}

// resolvePGHome determines the PostgreSQL home directory based on version or path
func resolvePGHome(arg string) (string, error) {
	// Check if it's a version number
	if ver, err := strconv.Atoi(arg); err == nil {
		if ver < minPGVersion || ver > maxPGVersion {
			return "", fmt.Errorf("invalid PostgreSQL version: %d (must be between %d and %d)",
				ver, minPGVersion, maxPGVersion)
		}

		switch config.OSType {
		case config.DistroEL:
			return fmt.Sprintf("/usr/pgsql-%d", ver), nil
		case config.DistroDEB:
			return fmt.Sprintf("/usr/lib/postgresql/%d", ver), nil
		default:
			return "", fmt.Errorf("unsupported OS distribution: %s", config.OSType)
		}
	}

	// Otherwise, treat as direct path
	return arg, nil
}

// validatePGHome checks if the given path is a valid PostgreSQL installation
func validatePGHome(pgHome string) error {
	if systemPaths[pgHome] {
		return fmt.Errorf("cannot use system path %s as PostgreSQL home", pgHome)
	}

	info, err := os.Stat(pgHome)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("PGHOME directory %s is not valid: %s", pgHome, err)
	}

	binDir := filepath.Join(pgHome, "bin")
	binInfo, err := os.Stat(binDir)
	if err != nil || !binInfo.IsDir() {
		return fmt.Errorf("bin directory %s is not valid: %s", binDir, err)
	}

	return nil
}

// generateProfile creates the profile script content
func generateProfile(pgHome, binDir string) []byte {
	var buf bytes.Buffer
	buf.WriteString("# generated by pig\n")
	buf.WriteString(fmt.Sprintf("export PATH=\"%s:$PATH\"\n", binDir))
	buf.WriteString(fmt.Sprintf("export PGHOME=\"%s\"\n", pgHome))
	return buf.Bytes()
}

// LinkPostgres links or unlinks PostgreSQL based on the given arguments.
// Usage:
// 1) LinkPostgres("none")             -> Unlink
// 2) LinkPostgres("14")               -> Link to system default path
// 3) LinkPostgres("/custom/pg/path")  -> Link to custom directory
func LinkPostgres(args ...string) error {
	if len(args) != 1 {
		return fmt.Errorf("exactly one argument required, got %d", len(args))
	}

	arg := args[0]
	if unlinkKeywords[arg] {
		return UnlinkPostgres()
	}

	// Resolve and validate PGHOME
	pgHome, err := resolvePGHome(arg)
	if err != nil {
		return err
	}

	if err := validatePGHome(pgHome); err != nil {
		logrus.Warnf("PGHOME directory %s is not valid: %s", pgHome, err)
		// TODO: can we check if the path is valid?
	}

	// Create symlink
	if err := RemoveSymlink(pgLinkPath); err != nil {
		return fmt.Errorf("failed to remove existing symlink: %w", err)
	}
	if err := utils.SudoCommand([]string{"ln", "-s", pgHome, pgLinkPath}); err != nil {
		return fmt.Errorf("failed to create symlink %s -> %s: %w", pgLinkPath, pgHome, err)
	}
	logrus.Infof("create symbolic link %s -> %s", pgLinkPath, pgHome)

	// Write profile
	binDir := filepath.Join(pgHome, "bin")
	if err := utils.PutFile(pgProfilePath, generateProfile(pgHome, binDir)); err != nil {
		return fmt.Errorf("failed to write profile file %s: %w", pgProfilePath, err)
	}

	logrus.Infof("write %s with PGHOME=%s and PATH+=%s", pgProfilePath, pgHome, binDir)
	logrus.Infof("run . %s to load the new env, run pig st to check active postgres", pgProfilePath)
	return nil
}
