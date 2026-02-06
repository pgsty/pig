/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pb stanza structured output result and DTO.
*/
package pgbackrest

import (
	"strings"

	"pig/internal/output"

	"github.com/sirupsen/logrus"
)

// PbStanzaResultData contains stanza operation result in a simplified, agent-friendly format.
// This struct is used as the Data field in output.Result for structured output of
// pb create, pb upgrade, and pb delete commands.
type PbStanzaResultData struct {
	Stanza    string `json:"stanza" yaml:"stanza"`                           // Stanza name
	Operation string `json:"operation" yaml:"operation"`                     // Operation type: create, upgrade, delete
	NoOnline  bool   `json:"no_online,omitempty" yaml:"no_online,omitempty"` // Offline mode (create/upgrade only)
	Force     bool   `json:"force,omitempty" yaml:"force,omitempty"`         // Force flag (create/delete only)
	Deleted   bool   `json:"deleted,omitempty" yaml:"deleted,omitempty"`     // Whether stanza was deleted (delete only)
}

// CreateResult creates a structured result for pb create (stanza-create) command.
// It validates configuration, executes stanza-create, and returns the result.
// Returns nil-safe Result on all paths.
func CreateResult(cfg *Config, opts *CreateOptions) *output.Result {
	if opts == nil {
		opts = &CreateOptions{}
	}
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return pbConfigErrorResult(err, output.CodePbStanzaCreateFailed, "Failed to get pgBackRest configuration")
	}

	var args []string
	if opts.NoOnline {
		args = append(args, "--no-online")
	}
	if opts.Force {
		args = append(args, "--force")
	}

	cmdOutput, cmdErr := RunPgBackRestOutput(effCfg, "stanza-create", args)
	if cmdErr != nil {
		errMsg := combineCommandError(cmdOutput, cmdErr)
		if isStanzaExistsMessage(errMsg) && !opts.Force {
			return output.Fail(output.CodePbStanzaExists, "Stanza already exists").
				WithDetail("Use --force to recreate the stanza")
		}
		if containsAny(errMsg, "permission denied", "Permission denied") {
			return output.Fail(output.CodePbPermissionDenied, "Permission denied during stanza create").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbStanzaCreateFailed, "Stanza create failed").
			WithDetail(errMsg)
	}

	if cmdOutput != "" {
		logrus.Debugf("pgbackrest stanza-create output: %s", cmdOutput)
	}

	data := &PbStanzaResultData{
		Stanza:    effCfg.Stanza,
		Operation: "create",
		NoOnline:  opts.NoOnline,
		Force:     opts.Force,
	}
	return output.OK("Stanza created successfully", data)
}

// UpgradeResult creates a structured result for pb upgrade (stanza-upgrade) command.
// It validates configuration, executes stanza-upgrade, and returns the result.
// Returns nil-safe Result on all paths.
func UpgradeResult(cfg *Config, opts *UpgradeOptions) *output.Result {
	if opts == nil {
		opts = &UpgradeOptions{}
	}
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return pbConfigErrorResult(err, output.CodePbStanzaUpgradeFailed, "Failed to get pgBackRest configuration")
	}

	var args []string
	if opts.NoOnline {
		args = append(args, "--no-online")
	}

	cmdOutput, cmdErr := RunPgBackRestOutput(effCfg, "stanza-upgrade", args)
	if cmdErr != nil {
		errMsg := combineCommandError(cmdOutput, cmdErr)
		if isStanzaNotFoundMessage(errMsg) {
			return output.Fail(output.CodePbStanzaNotFound, "Stanza does not exist").
				WithDetail("Create stanza first with: pig pb create")
		}
		if containsAny(errMsg, "permission denied", "Permission denied") {
			return output.Fail(output.CodePbPermissionDenied, "Permission denied during stanza upgrade").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbStanzaUpgradeFailed, "Stanza upgrade failed").
			WithDetail(errMsg)
	}

	if cmdOutput != "" {
		logrus.Debugf("pgbackrest stanza-upgrade output: %s", cmdOutput)
	}

	data := &PbStanzaResultData{
		Stanza:    effCfg.Stanza,
		Operation: "upgrade",
		NoOnline:  opts.NoOnline,
	}
	return output.OK("Stanza upgraded successfully", data)
}

// DeleteResult creates a structured result for pb delete (stanza-delete) command.
// It validates parameters and configuration, executes stanza-delete, and returns the result.
// Returns nil-safe Result on all paths.
//
// IMPORTANT: In structured output mode, --force is required as an explicit confirmation.
// The confirmation countdown is skipped (implicit --yes) since agents should have
// already confirmed their intent before calling delete.
func DeleteResult(cfg *Config, opts *DeleteOptions) *output.Result {
	if opts == nil {
		opts = &DeleteOptions{}
	}
	if !opts.Force {
		return output.Fail(output.CodePbStanzaDeleteRequiresForce, "Stanza delete requires --force flag").
			WithDetail("Use --force to confirm deletion of stanza and ALL its backups. This operation is IRREVERSIBLE.")
	}

	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return pbConfigErrorResult(err, output.CodePbStanzaDeleteFailed, "Failed to get pgBackRest configuration")
	}

	cmdOutput, cmdErr := RunPgBackRestOutput(effCfg, "stanza-delete", []string{"--force"})
	if cmdErr != nil {
		errMsg := combineCommandError(cmdOutput, cmdErr)
		if isStanzaNotFoundMessage(errMsg) {
			return output.Fail(output.CodePbStanzaNotFound, "Stanza does not exist").
				WithDetail("Cannot delete non-existent stanza")
		}
		if containsAny(errMsg, "permission denied", "Permission denied") {
			return output.Fail(output.CodePbPermissionDenied, "Permission denied during stanza delete").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbStanzaDeleteFailed, "Stanza delete failed").
			WithDetail(errMsg)
	}

	if cmdOutput != "" {
		logrus.Debugf("pgbackrest stanza-delete output: %s", cmdOutput)
	}

	data := &PbStanzaResultData{
		Stanza:    effCfg.Stanza,
		Operation: "delete",
		Force:     true, // Always true for delete (required parameter)
		Deleted:   true,
	}
	return output.OK("Stanza deleted successfully", data)
}

func pbConfigErrorResult(err error, fallbackCode int, fallbackMessage string) *output.Result {
	errMsg := err.Error()
	if containsAny(errMsg, "config file not found", "config file not accessible") {
		return output.Fail(output.CodePbConfigNotFound, "pgBackRest configuration not found").
			WithDetail(errMsg)
	}
	if containsAny(errMsg, "no stanza found", "cannot detect stanza") {
		return output.Fail(output.CodePbStanzaNotFound, "pgBackRest stanza not found").
			WithDetail(errMsg)
	}
	return output.Fail(fallbackCode, fallbackMessage).
		WithDetail(errMsg)
}

func isStanzaExistsMessage(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "stanza") && strings.Contains(lower, "exist")
}
