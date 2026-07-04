package pgbackrest

import (
	"fmt"
	"regexp"
	"strings"

	"pig/internal/output"
)

const pgBackRestBoundary = "pb:pgbackrest-only"

// expireDryRun runs pgbackrest expire --dry-run and returns its combined
// output. Injectable for tests. INFO console logging is forced so the preview
// detail is present regardless of the config file's console log level.
var expireDryRun = func(cfg *Config, opts *ExpireOptions) (string, error) {
	args := []string{"--dry-run", "--log-level-console=info"}
	if opts != nil && opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}
	return RunPgBackRestOutput(cfg, "expire", args)
}

// planConfig resolves the effective config for plan display so plans show the
// stanza and data directory execution would actually use. Resolution itself
// only reads the config file (any further plan-time work, like the expire
// dry-run, is the plan builder's own documented behavior). On failure the raw
// config is returned with resolved=false and plans degrade to placeholders.
func planConfig(cfg *Config) (*Config, bool) {
	eff, err := GetEffectiveConfig(cfg)
	if err != nil {
		if cfg == nil {
			return &Config{}, false
		}
		clone := *cfg
		return &clone, false
	}
	return eff, true
}

// commandConfig merges the user-provided config with the resolved stanza so
// rebuilt commands replay against a deterministic target: user flags
// (--config/--repo/--dbsu) are preserved verbatim, and the stanza is pinned
// from auto-detection when the user did not specify one. Pass a nil effCfg to
// skip pinning (e.g. when stanza selection is ambiguous).
func commandConfig(userCfg, effCfg *Config) *Config {
	out := &Config{}
	if userCfg != nil {
		*out = *userCfg
	}
	if out.Stanza == "" && effCfg != nil {
		out.Stanza = effCfg.Stanza
	}
	return out
}

func unresolvedConfigCheck() output.Check {
	return output.Check{
		Name:   "config resolution",
		Status: "unresolved",
		Detail: "pgBackRest config could not be read; stanza and data directory shown as defaults",
	}
}

// BuildRestorePlan returns a side-effect-free primitive plan for pgBackRest restore.
func BuildRestorePlan(cfg *Config, opts *RestoreOptions) *output.Plan {
	if opts == nil {
		opts = &RestoreOptions{}
	}
	effCfg, resolved := planConfig(cfg)
	cmdCfg := commandConfig(cfg, effCfg)
	normalizedTime := normalizeTime(opts.Time)
	// Commands render from the same normalization pass as the displayed
	// target, so a time-only input crossing midnight between two normalize
	// calls can never make the plan text and the replay command disagree.
	cmdOpts := *opts
	cmdOpts.Time = normalizedTime
	dataDir := getDataDir(effCfg, opts.DataDir)
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

	plan := &output.Plan{
		Command:      buildRestoreCommand(cmdCfg, &cmdOpts, true, false),
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
			{Type: "repository", Name: restoreRepoName(effCfg), Impact: "read", Detail: "backup source"},
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
			{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaName(effCfg)},
			{Name: "patroni lifecycle", Status: "not-managed", Detail: "pb restore will not pause, stop, or rejoin Patroni"},
		},
		Verifications: []output.Check{
			{Name: "restore command", Status: "pending", Detail: "pgBackRest restore exit status"},
			{Name: "post restore state", Status: "manual", Detail: "start and validate PostgreSQL explicitly"},
		},
		NextActions: []output.NextAction{
			{Command: buildRestoreCommand(cmdCfg, &cmdOpts, false, true), Reason: "execute low-level pgBackRest restore after explicit confirmation", Required: true},
			{Command: "pig pitr ... --plan", Reason: "orchestrated restore with PostgreSQL and Patroni lifecycle", Required: false},
			{Command: "pig pg status", Reason: "inspect local PostgreSQL state before and after restore", Required: false},
			{Command: infoCommand(cmdCfg), Reason: "inspect available backup sets", Required: false},
		},
	}
	if !resolved {
		plan.Preconditions = append(plan.Preconditions, unresolvedConfigCheck())
	}
	return plan
}

// BuildExpirePlan returns a non-deleting primitive plan for pgBackRest
// expire. When the config resolves, the plan embeds the native
// `pgbackrest expire --dry-run` output so structured mode is as informative
// as the text-mode dry-run. The dry-run executes pgBackRest as DBSU (reads
// the repository, may write pgBackRest logs/locks) but removes nothing.
func BuildExpirePlan(cfg *Config, opts *ExpireOptions) *output.Plan {
	if opts == nil {
		opts = &ExpireOptions{}
	}
	effCfg, resolved := planConfig(cfg)
	cmdCfg := commandConfig(cfg, effCfg)
	target := "retention policy"
	yesToExecute := false
	if opts.Set != "" {
		target = "backup set " + opts.Set
		yesToExecute = true // expire --set is gated on --yes
	}
	plan := &output.Plan{
		Command:      buildExpireCommand(cmdCfg, opts, true, false),
		Boundary:     pgBackRestBoundary,
		Confirmation: "recommended",
		Actions: []output.Action{
			{Step: 1, Description: "Resolve pgBackRest stanza and repository"},
			{Step: 2, Description: "Preview pgBackRest expire scope"},
			{Step: 3, Description: "Execute pgBackRest expire only when run without --plan"},
		},
		Affects: []output.Resource{
			{Type: "repository", Name: restoreRepoName(effCfg), Impact: "delete", Detail: target},
		},
		Expected: "Expired backups and WAL archives are removed by pgBackRest policy",
		Risks: []string{
			"Expired backup files and WAL archives may be permanently deleted.",
			"This primitive does not validate PostgreSQL or Patroni recovery posture.",
		},
		Preconditions: []output.Check{
			{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaName(effCfg)},
			{Name: "expire target", Status: "planned", Detail: target},
		},
		NextActions: []output.NextAction{
			{Command: buildExpireCommand(cmdCfg, opts, false, yesToExecute), Reason: "execute backup expiration", Required: true},
			{Command: infoCommand(cmdCfg), Reason: "verify retained backup sets", Required: false},
		},
	}
	if !resolved {
		plan.Preconditions = append(plan.Preconditions, unresolvedConfigCheck())
		return plan
	}
	if out, err := expireDryRun(effCfg, opts); err == nil {
		plan.DryRunOutput = strings.TrimSpace(out)
		plan.Verifications = append(plan.Verifications, output.Check{
			Name: "dry run", Status: "ok", Detail: "native pgbackrest expire --dry-run output embedded"})
	} else {
		plan.Verifications = append(plan.Verifications, output.Check{
			Name: "dry run", Status: "unavailable", Detail: strings.TrimSpace(combineCommandError(out, err))})
	}
	return plan
}

// BuildDeletePlan returns a side-effect-free primitive plan for pgBackRest stanza-delete.
func BuildDeletePlan(cfg *Config, opts *DeleteOptions) *output.Plan {
	if opts == nil {
		opts = &DeleteOptions{}
	}
	effCfg, resolved := planConfig(cfg)
	// Never pin an auto-detected stanza into delete commands when the config
	// defines several: an agent replaying the suggested command must not
	// delete a stanza the user never named.
	stanzas, ambiguityErr := RequireExplicitStanza(cfg)
	cmdCfg := commandConfig(cfg, effCfg)
	if ambiguityErr != nil {
		cmdCfg = commandConfig(cfg, nil)
	}
	plan := &output.Plan{
		Command:      buildDeleteCommand(cmdCfg, true, false),
		Boundary:     pgBackRestBoundary,
		Confirmation: "required",
		Actions: []output.Action{
			{Step: 1, Description: "Resolve pgBackRest stanza"},
			{Step: 2, Description: "Delete the stanza and all backups only when explicitly confirmed"},
		},
		Affects: []output.Resource{
			{Type: "stanza", Name: restoreStanzaName(effCfg), Impact: "delete", Detail: "all backups for stanza"},
			{Type: "repository", Name: restoreRepoName(effCfg), Impact: "delete", Detail: "stanza backup contents"},
		},
		Expected: "pgBackRest stanza and all associated backups are deleted",
		Risks: []string{
			"Stanza deletion is irreversible.",
			"All backups for the stanza are permanently removed.",
		},
		Preconditions: []output.Check{
			{Name: "explicit confirmation", Status: "required", Detail: "rerun with --yes for structured execution"},
			{Name: "pgbackrest stanza", Status: "required", Detail: restoreStanzaName(effCfg)},
		},
		NextActions: []output.NextAction{
			{Command: buildDeleteCommand(cmdCfg, false, true), Reason: "execute irreversible stanza deletion", Required: true},
			{Command: infoCommand(cmdCfg), Reason: "inspect backup inventory before deletion", Required: false},
		},
	}
	if !resolved {
		plan.Preconditions = append(plan.Preconditions, unresolvedConfigCheck())
	}
	if ambiguityErr != nil {
		// A blocked plan must not read as "this will delete <first stanza>".
		ambiguousDetail := "ambiguous: " + strings.Join(stanzas, ", ")
		for i := range plan.Affects {
			if plan.Affects[i].Type == "stanza" {
				plan.Affects[i].Name = "ambiguous"
			}
		}
		for i := range plan.Preconditions {
			if plan.Preconditions[i].Name == "pgbackrest stanza" {
				plan.Preconditions[i].Detail = ambiguousDetail
			}
		}
		plan.Preconditions = append(plan.Preconditions, output.Check{
			Name: "stanza selection", Status: "blocked", Detail: ambiguityErr.Error()})
		plan.NextActions = deletePlanPreviewActions(cfg, stanzas)
	}
	return plan
}

// deletePlanPreviewActions renders one per-stanza `pb delete --plan` preview,
// preserving the caller's --config/--repo/--dbsu so replays hit the same
// pgBackRest deployment the refusal was raised against.
func deletePlanPreviewActions(cfg *Config, stanzas []string) []output.NextAction {
	actions := make([]output.NextAction, 0, len(stanzas))
	for _, stanza := range stanzas {
		pinned := commandConfig(cfg, nil)
		pinned.Stanza = stanza
		actions = append(actions, output.NextAction{
			Command:  buildDeleteCommand(pinned, true, false),
			Reason:   "preview deletion scope for stanza " + stanza,
			Required: false,
		})
	}
	return actions
}

// RestoreCommand renders a replayable `pig pb restore` invocation including
// user-provided global flags (--stanza/--config/--repo/--dbsu); the stanza is
// pinned from auto-detection when unspecified so replays are deterministic.
func RestoreCommand(cfg *Config, opts *RestoreOptions, plan bool, yes bool) string {
	effCfg, _ := planConfig(cfg)
	return buildRestoreCommand(commandConfig(cfg, effCfg), opts, plan, yes)
}

// ExpireCommand renders a replayable `pig pb expire` invocation (see RestoreCommand).
func ExpireCommand(cfg *Config, opts *ExpireOptions, plan bool, yes bool) string {
	effCfg, _ := planConfig(cfg)
	return buildExpireCommand(commandConfig(cfg, effCfg), opts, plan, yes)
}

// DeleteCommand renders a replayable `pig pb delete` invocation (see
// RestoreCommand). Callers must refuse ambiguous stanza selection
// (RequireExplicitStanza) before suggesting this command, so pinning the
// auto-detected stanza here is only ever the single configured one.
func DeleteCommand(cfg *Config, plan bool, yes bool) string {
	effCfg, _ := planConfig(cfg)
	return buildDeleteCommand(commandConfig(cfg, effCfg), plan, yes)
}

// commandPrefix renders the pig pb invocation prefix with the config-level
// flags so rebuilt commands target the same stanza/config/repo as the
// original invocation.
func commandPrefix(cfg *Config, subcommand string) []string {
	parts := []string{"pig", "pb", subcommand}
	if cfg == nil {
		return parts
	}
	if cfg.Stanza != "" {
		parts = append(parts, "--stanza", quoteArg(cfg.Stanza))
	}
	if cfg.ConfigPath != "" && cfg.ConfigPath != DefaultConfigPath {
		parts = append(parts, "--config", quoteArg(cfg.ConfigPath))
	}
	if cfg.Repo != "" {
		parts = append(parts, "--repo", quoteArg(cfg.Repo))
	}
	if cfg.DbSU != "" {
		parts = append(parts, "--dbsu", quoteArg(cfg.DbSU))
	}
	return parts
}

func infoCommand(cfg *Config) string {
	return strings.Join(commandPrefix(cfg, "info"), " ")
}

func buildRestoreCommand(cfg *Config, opts *RestoreOptions, includePlan bool, includeYes bool) string {
	parts := commandPrefix(cfg, "restore")
	if opts.Default {
		parts = append(parts, "--default")
	}
	if opts.Immediate {
		parts = append(parts, "--immediate")
	}
	if opts.Time != "" {
		// Normalized (timezone-completed, date-anchored) so replaying the
		// command later or elsewhere hits the same recovery point: a bare
		// "12:00:00" would otherwise re-resolve to the replay day.
		parts = append(parts, "--time", quoteArg(normalizeTime(opts.Time)))
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
	if opts.TargetAction != "" {
		parts = append(parts, "--target-action", quoteArg(opts.TargetAction))
	}
	if opts.TargetTimeline != "" {
		parts = append(parts, "--target-timeline", quoteArg(opts.TargetTimeline))
	}
	if includeYes {
		parts = append(parts, "--yes")
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

func buildExpireCommand(cfg *Config, opts *ExpireOptions, includePlan bool, includeYes bool) string {
	parts := commandPrefix(cfg, "expire")
	if opts != nil && opts.Set != "" {
		parts = append(parts, "--set", quoteArg(opts.Set))
	}
	if includeYes {
		parts = append(parts, "--yes")
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

func buildDeleteCommand(cfg *Config, includePlan bool, includeYes bool) string {
	parts := commandPrefix(cfg, "delete")
	if includeYes {
		parts = append(parts, "--yes")
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

// shellSafeArgRegex matches arguments that need no quoting in a replayable
// shell command.
var shellSafeArgRegex = regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`)

// quoteArg renders one argument shell-safe for replayable commands using
// POSIX single-quote escaping. Double quotes are not enough: globs (*),
// $-expansion, and backticks would still be interpreted on replay (the docs
// showcase --set 20250101-* which must survive verbatim).
func quoteArg(arg string) string {
	if shellSafeArgRegex.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// QuoteShellArg renders one shell-safe argument using the same POSIX quoting
// contract as replayable pgBackRest plans.
func QuoteShellArg(value string) string {
	return quoteArg(value)
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
