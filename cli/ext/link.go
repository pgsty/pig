package ext

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	pgLinkPath    = "/usr/pgsql"
	pgProfilePath = "/etc/profile.d/pg.sh"
)

// LinkPostgres links PostgreSQL installation to system paths
func LinkPostgres(pgVer int, pgConfigPath string) error {
	logrus.Debugf("linking postgres: pgVer=%d, pgConfigPath=%s", pgVer, pgConfigPath)

	// Get PostgreSQL installation
	if err := DetectPostgres(); err != nil {
		logrus.Debugf("failed to detect PostgreSQL: %v", err)
	}

	var pg *PostgresInstall
	var err error
	if pgConfigPath != "" {
		pg, err = GetPostgres(pgConfigPath)
	} else if pgVer != 0 {
		pg, err = GetPostgres(fmt.Sprintf("%d", pgVer))
	} else if Active != nil {
		pg = Active
	} else {
		return fmt.Errorf("no PostgreSQL installation specified or found")
	}

	if err != nil {
		return fmt.Errorf("failed to get PostgreSQL installation: %v", err)
	}

	// Create symbolic link
	if err := os.RemoveAll(pgLinkPath); err != nil {
		return fmt.Errorf("failed to remove existing link: %v", err)
	}

	pgHome := filepath.Dir(filepath.Dir(pg.BinPath))
	if err := os.Symlink(pgHome, pgLinkPath); err != nil {
		return fmt.Errorf("failed to create symbolic link: %v", err)
	}
	logrus.Infof("linked %s -> %s", pgLinkPath, pgHome)

	// Create profile script
	profileContent := fmt.Sprintf(`# PostgreSQL %d PATH
export PATH=%s/bin:$PATH
`, pg.MajorVersion, pgLinkPath)

	if err := os.WriteFile(pgProfilePath, []byte(profileContent), 0644); err != nil {
		return fmt.Errorf("failed to write profile script: %v", err)
	}
	logrus.Infof("created profile script: %s", pgProfilePath)

	// Set ownership
	cmds := []string{
		fmt.Sprintf("chown -h root:root %s", pgLinkPath),
		fmt.Sprintf("chown root:root %s", pgProfilePath),
	}
	for _, cmd := range cmds {
		if err := utils.SudoCommand(strings.Split(cmd, " ")); err != nil {
			return fmt.Errorf("failed to set ownership: %v", err)
		}
	}

	logrus.Infof("PostgreSQL %d is now linked to system paths", pg.MajorVersion)
	logrus.Infof("Please run 'source %s' to update your PATH", pgProfilePath)
	return nil
}
