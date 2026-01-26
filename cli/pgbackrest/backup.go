package pgbackrest

import (
	"fmt"

	"pig/cli/postgres"

	"github.com/sirupsen/logrus"
)

// Valid backup types
var validBackupTypes = map[string]bool{
	"full": true,
	"diff": true,
	"incr": true,
}

// BackupOptions holds options for backup command.
type BackupOptions struct {
	Type  string // Backup type: full, diff, incr (empty = auto)
	Force bool   // Skip primary role check
}

// Backup creates a physical backup.
// Backup can only run on the primary instance.
// Note: pgBackRest automatically determines backup type if not specified:
//   - If no full backup exists: performs full backup
//   - Otherwise: performs incremental backup
func Backup(cfg *Config, opts *BackupOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	// Validate backup type
	if opts.Type != "" && !validBackupTypes[opts.Type] {
		return fmt.Errorf("invalid backup type: %s (use: full, diff, incr)", opts.Type)
	}

	// Check primary role (unless --force)
	if !opts.Force {
		if err := checkPrimaryRole(); err != nil {
			return err
		}
	}

	// Build command arguments
	var args []string
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
	}

	return RunPgBackRest(effCfg, "backup", args, true)
}

// checkPrimaryRole verifies current instance is primary
func checkPrimaryRole() error {
	// Use postgres package role detection
	pgConfig := postgres.DefaultConfig()
	roleResult, err := postgres.DetectRole(pgConfig, &postgres.RoleOptions{
		Verbose: false,
	})

	if err != nil {
		logrus.Warnf("cannot detect PostgreSQL role: %v", err)
		logrus.Warnf("use --force to skip this check")
		return fmt.Errorf("cannot verify primary role: %w", err)
	}

	switch roleResult.Role {
	case postgres.RolePrimary:
		logrus.Infof("confirmed running on primary instance (source: %s)", roleResult.Source)
		return nil
	case postgres.RoleReplica:
		return fmt.Errorf("backup should run on primary instance, current is replica (source: %s)", roleResult.Source)
	default:
		if !roleResult.Alive {
			return fmt.Errorf("PostgreSQL is not running, cannot perform backup")
		}
		logrus.Warnf("cannot determine instance role (source: %s)", roleResult.Source)
		return fmt.Errorf("cannot confirm primary role, use --force to override")
	}
}

// ExpireOptions holds options for expire command.
type ExpireOptions struct {
	Set    string // Specific backup set to delete
	DryRun bool   // Dry-run mode: only show what would be deleted
}

// Expire cleans up expired backups according to retention policy.
func Expire(cfg *Config, opts *ExpireOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	var args []string
	if opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}

	return RunPgBackRest(effCfg, "expire", args, true)
}
