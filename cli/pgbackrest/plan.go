package pgbackrest

import (
	"fmt"
	"strconv"
	"strings"

	"pig/internal/output"
)

const pgBackRestBoundary = "pb:pgbackrest-only"

// BuildRestorePlan returns a side-effect-free primitive plan for pgBackRest restore.
func BuildRestorePlan(cfg *Config, opts *RestoreOptions) *output.Plan {
	if opts == nil {
		opts = &RestoreOptions{}
	}
	normalizedTime := normalizeTime(opts.Time)
	dataDir := getDataDir(cfg, opts.DataDir)
	targetType := determineTargetType(opts)
	targetValue := determineTargetValue(opts, normalizedTime)
	targetDetail := targetType
	if targetValue != "" {
		targetDetail = fmt.Sprintf("%s=%s", targetType, targetValue)
	}
	if opts.Set != "" {
		targetDetail = strings.TrimSpace(targetDetail + " backup_set=" + opts.Set)
	}
	if action := determineTargetAction(opts); action != "" {
		targetDetail = strings.TrimSpace(targetDetail + " target_action=" + action)
	}
	if opts.TargetTimeline != "" {
		targetDetail = strings.TrimSpace(targetDetail + " target_timeline=" + opts.TargetTimeline)
	}

	return &output.Plan{
		Command:      buildRestoreCommand(opts, true),
		Boundary:     pgBackRestBoundary,
		Confirmation: "required",
		Actions: []output.Action{
			{Step: 1, Description: "Validate restore target and pgBackRest parameters"},
			{Step: 2, Description: "Check that the target PostgreSQL data directory is not running"},
			{Step: 3, Description: "Execute pgBackRest restore only; do not stop/start PostgreSQL or Patroni"},
			{Step: 4, Description: "Leave post-restore startup, promotion, and HA validation to the caller"},
		},
		Affects: []output.Resource{
			{Type: "data_dir", Name: dataDir, Impact: "overwrite", Detail: "pgBackRest restore target"},
			{Type: "repository", Name: restoreRepoName(cfg), Impact: "read", Detail: "backup source"},
			{Type: "service", Name: "patroni/postgresql", Impact: "not-managed", Detail: "primitive boundary does not manage lifecycle"},
		},
		Expected: "pgBackRest restore completes; PostgreSQL and Patroni lifecycle remain unchanged",
		Risks: []string{
			"Restore can overwrite the target PostgreSQL data directory.",
			"This primitive does not stop Patroni, stop PostgreSQL, start PostgreSQL, promote, or verify HA state.",
			"Use pig pitr for orchestrated recovery on managed Pigsty clusters.",
		},
		Preconditions: []output.Check{
			{Name: "recovery target", Status: "required", Detail: targetDetail},
			{Name: "postgres stopped", Status: "required", Detail: fmt.Sprintf("%s must not be running", dataDir)},
			{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaName(cfg)},
			{Name: "patroni lifecycle", Status: "not-managed", Detail: "pb restore will not pause, stop, or rejoin Patroni"},
		},
		Verifications: []output.Check{
			{Name: "restore command", Status: "pending", Detail: "pgBackRest restore exit status"},
			{Name: "post restore state", Status: "manual", Detail: "start and validate PostgreSQL explicitly"},
		},
		NextActions: []output.NextAction{
			{Command: "pig pitr ... --plan", Reason: "orchestrated restore with PostgreSQL and Patroni lifecycle", Required: false},
			{Command: "pig pg status", Reason: "inspect local PostgreSQL state before and after restore", Required: false},
			{Command: "pig pb info", Reason: "inspect available backup sets", Required: false},
		},
	}
}

// BuildExpirePlan returns a side-effect-free primitive plan for pgBackRest expire.
func BuildExpirePlan(cfg *Config, opts *ExpireOptions) *output.Plan {
	if opts == nil {
		opts = &ExpireOptions{}
	}
	target := "retention policy"
	if opts.Set != "" {
		target = "backup set " + opts.Set
	}
	return &output.Plan{
		Command:      buildExpireCommand(opts, true),
		Boundary:     pgBackRestBoundary,
		Confirmation: "recommended",
		Actions: []output.Action{
			{Step: 1, Description: "Resolve pgBackRest stanza and repository"},
			{Step: 2, Description: "Preview pgBackRest expire scope"},
			{Step: 3, Description: "Execute pgBackRest expire only when run without --plan"},
		},
		Affects: []output.Resource{
			{Type: "repository", Name: restoreRepoName(cfg), Impact: "delete", Detail: target},
		},
		Expected: "Expired backups and WAL archives are removed by pgBackRest policy",
		Risks: []string{
			"Expired backup files and WAL archives may be permanently deleted.",
			"This primitive does not validate PostgreSQL or Patroni recovery posture.",
		},
		Preconditions: []output.Check{
			{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaName(cfg)},
			{Name: "expire target", Status: "planned", Detail: target},
		},
		NextActions: []output.NextAction{
			{Command: "pig pb expire --plan", Reason: "preview expire scope before deleting backups", Required: false},
			{Command: "pig pb info", Reason: "verify retained backup sets", Required: false},
		},
	}
}

// BuildDeletePlan returns a side-effect-free primitive plan for pgBackRest stanza-delete.
func BuildDeletePlan(cfg *Config, opts *DeleteOptions) *output.Plan {
	if opts == nil {
		opts = &DeleteOptions{}
	}
	return &output.Plan{
		Command:      "pig pb delete --plan",
		Boundary:     pgBackRestBoundary,
		Confirmation: "required",
		Actions: []output.Action{
			{Step: 1, Description: "Resolve pgBackRest stanza"},
			{Step: 2, Description: "Delete the stanza and all backups only when explicitly confirmed"},
		},
		Affects: []output.Resource{
			{Type: "stanza", Name: restoreStanzaName(cfg), Impact: "delete", Detail: "all backups for stanza"},
			{Type: "repository", Name: restoreRepoName(cfg), Impact: "delete", Detail: "stanza backup contents"},
		},
		Expected: "pgBackRest stanza and all associated backups are deleted",
		Risks: []string{
			"Stanza deletion is irreversible.",
			"All backups for the stanza are permanently removed.",
		},
		Preconditions: []output.Check{
			{Name: "explicit confirmation", Status: "required", Detail: "rerun with --force for structured execution"},
			{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaName(cfg)},
		},
		NextActions: []output.NextAction{
			{Command: "pig pb delete --force", Reason: "execute irreversible stanza deletion", Required: true},
			{Command: "pig pb info", Reason: "inspect backup inventory before deletion", Required: false},
		},
	}
}

func buildRestoreCommand(opts *RestoreOptions, includePlan bool) string {
	parts := []string{"pig", "pb", "restore"}
	if opts.Default {
		parts = append(parts, "--default")
	}
	if opts.Immediate {
		parts = append(parts, "--immediate")
	}
	if opts.Time != "" {
		parts = append(parts, "--time", quoteArg(opts.Time))
	}
	if opts.Name != "" {
		parts = append(parts, "--name", quoteArg(opts.Name))
	}
	if opts.LSN != "" {
		parts = append(parts, "--lsn", quoteArg(opts.LSN))
	}
	if opts.XID != "" {
		parts = append(parts, "--xid", quoteArg(opts.XID))
	}
	if opts.Set != "" {
		parts = append(parts, "--set", quoteArg(opts.Set))
	}
	if opts.DataDir != "" {
		parts = append(parts, "--data", quoteArg(opts.DataDir))
	}
	if opts.Exclusive {
		parts = append(parts, "--exclusive")
	}
	if opts.Promote {
		parts = append(parts, "--promote")
	}
	if opts.TargetAction != "" {
		parts = append(parts, "--target-action", quoteArg(opts.TargetAction))
	}
	if opts.TargetTimeline != "" {
		parts = append(parts, "--target-timeline", quoteArg(opts.TargetTimeline))
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	if len(opts.ExtraArgs) > 0 {
		parts = append(parts, "--")
		for _, arg := range opts.ExtraArgs {
			parts = append(parts, quoteArg(arg))
		}
	}
	return strings.Join(parts, " ")
}

func buildExpireCommand(opts *ExpireOptions, includePlan bool) string {
	parts := []string{"pig", "pb", "expire"}
	if opts.Set != "" {
		parts = append(parts, "--set", quoteArg(opts.Set))
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return arg
	}
	if strings.ContainsAny(arg, " \t\n\"'\\") {
		return strconv.Quote(arg)
	}
	return arg
}

func restoreRepoName(cfg *Config) string {
	if cfg != nil && cfg.Repo != "" {
		return "repo" + cfg.Repo
	}
	return "repo1"
}

func restoreStanzaName(cfg *Config) string {
	if cfg != nil && cfg.Stanza != "" {
		return cfg.Stanza
	}
	return "auto-detect"
}
