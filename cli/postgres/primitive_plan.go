package postgres

import (
	"fmt"
	"strconv"
	"strings"

	"pig/internal/output"
)

const pgLocalBoundary = "pg:local-instance"

// BuildPromotePlan returns a side-effect-free primitive plan for pg promote.
func BuildPromotePlan(cfg *Config, opts *PromoteOptions) *output.Plan {
	dataDir := GetPgData(cfg)
	dbsu := GetDbSU(cfg)
	running, pid := CheckPostgresRunningAsDBSU(dbsu, dataDir)
	role := detectRoleString(cfg)

	return &output.Plan{
		Command:      buildPromoteCommand(opts, true),
		Boundary:     pgLocalBoundary,
		Confirmation: "required",
		Actions: []output.Action{
			{Step: 1, Description: "Verify the local PostgreSQL instance is a running standby"},
			{Step: 2, Description: "Execute pg_ctl promote on the local data directory"},
			{Step: 3, Description: "Verify local role changed to primary"},
		},
		Affects: []output.Resource{
			{Type: "data_dir", Name: dataDir, Impact: "timeline change", Detail: "local PostgreSQL promotion"},
			{Type: "service", Name: "patroni", Impact: "not-managed", Detail: "pg promote does not coordinate Patroni cluster state"},
		},
		Expected: "Local standby is promoted to primary; Patroni and cluster routing are not coordinated by this primitive",
		Risks: []string{
			"Promoting outside Patroni can create split-brain if the cluster is still managed by Patroni.",
			"Clients and replicas are not redirected by this primitive.",
			"Use pig pt switchover/failover or pig pitr for managed cluster orchestration.",
		},
		Preconditions: []output.Check{
			{Name: "data_dir", Status: "planned", Detail: dataDir},
			{Name: "postgres running", Status: boolStatus(running), Detail: fmt.Sprintf("pid=%d", pid)},
			{Name: "current role", Status: roleStatus(role), Detail: role},
			{Name: "boundary", Status: "local-only", Detail: "does not manage Patroni, DCS, VIP, or client routing"},
		},
		Verifications: []output.Check{
			{Name: "role", Status: "manual", Detail: "pig pg role"},
			{Name: "status", Status: "manual", Detail: "pig pg status"},
		},
		NextActions: []output.NextAction{
			{Command: "pig pt switchover --plan", Reason: "planned Patroni-managed leadership transfer", Required: false},
			{Command: "pig pt failover --plan", Reason: "Patroni-managed emergency leadership transfer", Required: false},
			{Command: "pig pg promote --yes", Reason: "execute local-only promotion after explicit confirmation", Required: true},
		},
	}
}

// BuildRepackPlan returns a side-effect-free primitive plan for pg repack.
func BuildRepackPlan(cfg *Config, dbname string, opts *RepackOptions) *output.Plan {
	target := maintenanceTarget(dbname, opts)
	jobs := 1
	if opts != nil && opts.Jobs > 0 {
		jobs = opts.Jobs
	}
	return &output.Plan{
		Command:      buildRepackCommand(dbname, opts, true),
		Boundary:     pgLocalBoundary,
		Confirmation: "recommended",
		Actions: []output.Action{
			{Step: 1, Description: "Validate repack database/schema/table target"},
			{Step: 2, Description: "Run pg_repack on the local PostgreSQL instance when executed"},
			{Step: 3, Description: "Verify table bloat and session impact after repack"},
		},
		Affects: []output.Resource{
			{Type: "database", Name: target, Impact: "rewrite", Detail: fmt.Sprintf("pg_repack jobs=%d", jobs)},
		},
		Expected: "Selected relations are rebuilt online by pg_repack on the local PostgreSQL instance",
		Risks: []string{
			"pg_repack takes locks at the beginning and end of table rebuilds.",
			"Large tables can create heavy IO and replication lag.",
			"This primitive does not coordinate Patroni, traffic draining, or maintenance windows.",
		},
		Preconditions: []output.Check{
			{Name: "target", Status: "planned", Detail: target},
			{Name: "extension", Status: "required", Detail: "pg_repack must be installed and usable"},
			{Name: "boundary", Status: "local-only", Detail: "does not manage Patroni or application traffic"},
		},
		Verifications: []output.Check{
			{Name: "sessions", Status: "manual", Detail: "pig pg ps"},
			{Name: "repack result", Status: "manual", Detail: "inspect pg_repack native output"},
		},
		NextActions: []output.NextAction{
			{Command: strings.TrimSuffix(buildRepackCommand(dbname, opts, false), " "), Reason: "execute repack after reviewing the plan", Required: true},
			{Command: "pig ext add pg_repack", Reason: "install pg_repack if missing", Required: false},
		},
	}
}

// BuildVacuumPlan returns a side-effect-free primitive plan for pg vacuum.
func BuildVacuumPlan(cfg *Config, dbname string, opts *VacuumOptions) *output.Plan {
	target := vacuumTarget(dbname, opts)
	mode := "VACUUM"
	confirmation := "none"
	risks := []string{"This primitive does not coordinate Patroni, traffic draining, or maintenance windows."}
	if opts != nil && opts.Full {
		mode = "VACUUM FULL"
		confirmation = "recommended"
		risks = append(risks,
			"VACUUM FULL rewrites relations and requires exclusive locks.",
			"Large relations can create heavy IO and block application queries.",
		)
	}
	return &output.Plan{
		Command:      buildVacuumCommand(dbname, opts, true),
		Boundary:     pgLocalBoundary,
		Confirmation: confirmation,
		Actions: []output.Action{
			{Step: 1, Description: "Validate vacuum database/schema/table target"},
			{Step: 2, Description: "Run " + mode + " on the local PostgreSQL instance when executed"},
			{Step: 3, Description: "Verify session and table health after vacuum"},
		},
		Affects: []output.Resource{
			{Type: "database", Name: target, Impact: strings.ToLower(mode), Detail: "local PostgreSQL maintenance"},
		},
		Expected: mode + " completes for the selected target",
		Risks:    risks,
		Preconditions: []output.Check{
			{Name: "target", Status: "planned", Detail: target},
			{Name: "boundary", Status: "local-only", Detail: "does not manage Patroni or application traffic"},
		},
		Verifications: []output.Check{
			{Name: "sessions", Status: "manual", Detail: "pig pg ps"},
		},
		NextActions: []output.NextAction{
			{Command: buildVacuumCommand(dbname, opts, false), Reason: "execute vacuum after reviewing the plan", Required: opts != nil && opts.Full},
		},
	}
}

// ValidateMaintenanceOptions exposes the same maintenance validation used by execution paths.
func ValidateMaintenanceOptions(schema, table string) error {
	return validateMaintOptions(schema, table)
}

func buildPromoteCommand(opts *PromoteOptions, includePlan bool) string {
	parts := []string{"pig", "pg", "promote"}
	if opts != nil && opts.Timeout > 0 {
		parts = append(parts, "--timeout", strconv.Itoa(opts.Timeout))
	}
	if opts != nil && opts.NoWait {
		parts = append(parts, "--no-wait")
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

func buildRepackCommand(dbname string, opts *RepackOptions, includePlan bool) string {
	parts := []string{"pig", "pg", "repack"}
	if dbname != "" {
		parts = append(parts, quotePgArg(dbname))
	}
	if opts != nil {
		appendMaintOptions(&parts, opts.All, opts.Schema, opts.Table, opts.Verbose)
		if opts.Jobs > 1 {
			parts = append(parts, "--jobs", strconv.Itoa(opts.Jobs))
		}
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

func buildVacuumCommand(dbname string, opts *VacuumOptions, includePlan bool) string {
	parts := []string{"pig", "pg", "vacuum"}
	if dbname != "" {
		parts = append(parts, quotePgArg(dbname))
	}
	if opts != nil {
		appendMaintOptions(&parts, opts.All, opts.Schema, opts.Table, opts.Verbose)
		if opts.Full {
			parts = append(parts, "--full")
		}
	}
	if includePlan {
		parts = append(parts, "--plan")
	}
	return strings.Join(parts, " ")
}

func appendMaintOptions(parts *[]string, all bool, schema, table string, verbose bool) {
	if all {
		*parts = append(*parts, "--all")
	}
	if schema != "" {
		*parts = append(*parts, "--schema", quotePgArg(schema))
	}
	if table != "" {
		*parts = append(*parts, "--table", quotePgArg(table))
	}
	if verbose {
		*parts = append(*parts, "--verbose")
	}
}

func maintenanceTarget(dbname string, opts *RepackOptions) string {
	if opts != nil && opts.All {
		return "all databases"
	}
	if dbname == "" {
		dbname = "postgres"
	}
	if opts == nil {
		return dbname
	}
	return relationTarget(dbname, opts.Schema, opts.Table)
}

func vacuumTarget(dbname string, opts *VacuumOptions) string {
	if opts != nil && opts.All {
		return "all databases"
	}
	if dbname == "" {
		dbname = "postgres"
	}
	if opts == nil {
		return dbname
	}
	return relationTarget(dbname, opts.Schema, opts.Table)
}

func relationTarget(dbname, schema, table string) string {
	if table != "" && schema != "" {
		return fmt.Sprintf("%s.%s.%s", dbname, schema, table)
	}
	if table != "" {
		return fmt.Sprintf("%s.%s", dbname, table)
	}
	if schema != "" {
		return fmt.Sprintf("%s schema=%s", dbname, schema)
	}
	return dbname
}

func boolStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "not-ok"
}

func roleStatus(role string) string {
	if role == "replica" || role == "standby" {
		return "ok"
	}
	if role == "primary" {
		return "not-ok"
	}
	return "unknown"
}
