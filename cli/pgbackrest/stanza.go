package pgbackrest

import (
	"fmt"
	"os"

	"pig/internal/utils"
)

// CreateOptions holds options for stanza-create command.
type CreateOptions struct {
	NoOnline bool // Create stanza without PostgreSQL running
	Force    bool // Force creation
}

// Create initializes a new stanza (stanza-create).
// Must be run before the first backup.
func Create(cfg *Config, opts *CreateOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	var args []string
	if opts.NoOnline {
		args = append(args, "--no-online")
	}
	if opts.Force {
		args = append(args, "--force")
	}

	return RunPgBackRest(effCfg, "stanza-create", args, true)
}

// UpgradeOptions holds options for stanza-upgrade command.
type UpgradeOptions struct {
	NoOnline bool // Upgrade stanza without PostgreSQL running
}

// Upgrade updates stanza after PostgreSQL major version upgrade (stanza-upgrade).
func Upgrade(cfg *Config, opts *UpgradeOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	var args []string
	if opts.NoOnline {
		args = append(args, "--no-online")
	}

	return RunPgBackRest(effCfg, "stanza-upgrade", args, true)
}

// DeleteOptions holds options for stanza-delete command.
type DeleteOptions struct {
	Force bool // Force deletion (required)
	Yes   bool // Skip countdown confirmation
}

// Delete removes a stanza and all its backups (stanza-delete).
// WARNING: This is a destructive and irreversible operation!
func Delete(cfg *Config, opts *DeleteOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	if !opts.Force {
		return fmt.Errorf("stanza-delete is a destructive operation that removes ALL backups\nuse --force to confirm deletion of stanza '%s'", effCfg.Stanza)
	}

	if !opts.Yes {
		fmt.Fprintf(os.Stderr, "\n%s!!! WARNING !!!%s\n", utils.ColorRed, utils.ColorReset)
		fmt.Fprintf(os.Stderr, "This will permanently delete stanza '%s' and ALL its backups.\n", effCfg.Stanza)
		fmt.Fprintf(os.Stderr, "This operation is %sIRREVERSIBLE%s.\n", utils.ColorRed, utils.ColorReset)

		if err := ConfirmWithCountdown("stanza deletion", "stanza deletion"); err != nil {
			return err
		}
	}

	return RunPgBackRest(effCfg, "stanza-delete", []string{"--force"}, true)
}
