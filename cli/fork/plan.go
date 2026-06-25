package fork

import (
	"fmt"
	"strings"

	"pig/internal/output"
)

func BuildPlan(opts *Options, state *State) *output.Plan {
	if opts == nil {
		return &output.Plan{Command: "pig pg fork"}
	}
	switch opts.Kind {
	case KindInstance:
		return buildInstancePlan(opts, state)
	case KindDatabase:
		return buildDatabasePlan(opts, state)
	default:
		return &output.Plan{Command: BuildCommand(opts)}
	}
}

func buildInstancePlan(opts *Options, state *State) *output.Plan {
	inst := opts.Instance
	backupMode := BackupModeHot
	cloneMode := CloneModeCopy
	if state != nil {
		if state.BackupMode != "" && state.BackupMode != BackupModeUnknown {
			backupMode = state.BackupMode
		}
		if state.CloneMode != "" && state.CloneMode != CloneModeUnknown {
			cloneMode = state.CloneMode
		}
	}

	actions := []output.Action{}
	step := 1
	if backupMode == BackupModeHot {
		actions = append(actions, output.Action{Step: step, Description: "Start PostgreSQL backup mode"})
		step++
	} else {
		actions = append(actions, output.Action{Step: step, Description: "Use cold copy mode"})
		step++
	}
	copyDesc := "Clone data directory"
	if cloneMode == CloneModeCOW {
		copyDesc = "Clone data directory with CoW"
	}
	actions = append(actions,
		output.Action{Step: step, Description: copyDesc},
		output.Action{Step: step + 1, Description: "Prepare forked instance configuration"},
	)
	step += 2
	if opts.Start {
		actions = append(actions, output.Action{Step: step, Description: "Start forked PostgreSQL instance"})
		step++
		actions = append(actions, output.Action{Step: step, Description: "Verify forked instance is reachable"})
	}

	risks := []string{"Destination data directory will be removed when --force is used"}
	if cloneMode == CloneModeCOW {
		risks = append(risks, "Copy-on-write forks share physical blocks until either side writes")
	} else {
		risks = append(risks, "Execution requires verified CoW support; use --force to allow regular copy fallback")
	}
	if backupMode == BackupModeCold {
		risks = append(risks, "Cold copy requires the source instance to be stopped")
	}

	return &output.Plan{
		Command: BuildCommand(opts),
		Actions: actions,
		Affects: []output.Resource{
			{Type: "instance", Name: inst.SourceData, Impact: "read", Detail: fmt.Sprintf("port %d", inst.SourcePort)},
			{Type: "instance", Name: inst.DestData, Impact: "create", Detail: fmt.Sprintf("port %d", inst.DestPort)},
		},
		Expected: fmt.Sprintf("PostgreSQL instance forked from %s to %s on port %d", inst.SourceData, inst.DestData, inst.DestPort),
		Risks:    risks,
	}
}

func buildDatabasePlan(opts *Options, state *State) *output.Plan {
	db := opts.Database
	actions := []output.Action{}
	step := 1
	if db.Kill {
		actions = append(actions, output.Action{Step: step, Description: fmt.Sprintf("Terminate existing connections to %s", db.SourceDB)})
		step++
	}
	actions = append(actions, output.Action{
		Step:        step,
		Description: fmt.Sprintf("Create database %s from template %s", db.DestDB, db.SourceDB),
	})

	risks := []string{
		"CREATE DATABASE from template requires no active connections on the source database",
	}
	if db.Kill {
		risks = append(risks, "Active source database sessions will be terminated")
	}
	if state != nil && state.CloneMode == CloneModeCopy {
		risks = append(risks, "Database copy may fall back to regular file copy if clone support is unavailable")
	}

	return &output.Plan{
		Command: BuildCommand(opts),
		Actions: actions,
		Affects: []output.Resource{
			{Type: "database", Name: db.SourceDB, Impact: "read", Detail: fmt.Sprintf("port %d", db.Port)},
			{Type: "database", Name: db.DestDB, Impact: "create", Detail: db.Strategy},
		},
		Expected: fmt.Sprintf("Database %s cloned from %s using %s", db.DestDB, db.SourceDB, db.Strategy),
		Risks:    risks,
	}
}

func BuildCommand(opts *Options) string {
	if opts == nil {
		return "pig pg fork"
	}
	args := []string{"pig", "pg"}
	switch opts.Kind {
	case KindInstance:
		args = append(args, "fork", opts.Instance.Name)
		if opts.Instance.SourceData != "" && opts.Instance.SourceData != "/pg/data" {
			args = append(args, "-D", quoteArg(opts.Instance.SourceData))
		}
		if opts.Instance.SourcePort != 0 && opts.Instance.SourcePort != 5432 {
			args = append(args, "-P", fmt.Sprintf("%d", opts.Instance.SourcePort))
		}
		if opts.Instance.DestData != "" && opts.Instance.DestData != "/pg/data-"+opts.Instance.Name {
			args = append(args, "-d", quoteArg(opts.Instance.DestData))
		}
		if opts.Instance.DestPort != 0 && opts.Instance.DestPort != 15432 {
			args = append(args, "-p", fmt.Sprintf("%d", opts.Instance.DestPort))
		}
		if opts.Run {
			args = append(args, "-r")
		}
		if opts.Replace {
			args = append(args, "-f")
		}
	case KindDatabase:
		args = append(args, "clone", opts.Database.SourceDB, opts.Database.DestDB)
		if opts.Database.NoKill {
			args = append(args, "--no-kill")
		}
		if opts.Database.Port != 0 && opts.Database.Port != 5432 {
			args = append(args, "-p", fmt.Sprintf("%d", opts.Database.Port))
		}
		if opts.Database.ConnDB != "" && opts.Database.ConnDB != "postgres" {
			args = append(args, "--conn-db", quoteArg(opts.Database.ConnDB))
		}
		if opts.Database.Strategy != "" && opts.Database.Strategy != "FILE_COPY" {
			args = append(args, "--strategy", opts.Database.Strategy)
		}
	}
	if opts.Yes {
		args = append(args, "-y")
	}
	if opts.Plan {
		args = append(args, "--plan")
	}
	return strings.Join(args, " ")
}

func BuildDatabaseCloneSQL(opts *DatabaseOptions) string {
	if opts == nil {
		return ""
	}
	strategy := opts.Strategy
	if strategy == "" {
		strategy = "FILE_COPY"
	}
	strategy = strings.ToUpper(strings.ReplaceAll(strategy, "-", "_"))
	var sb strings.Builder
	sb.WriteString("\\set ON_ERROR_STOP on\n")
	if shouldKillDatabaseConnections(*opts) {
		sb.WriteString("SELECT pg_terminate_backend(pid)\n")
		sb.WriteString("  FROM pg_stat_activity\n")
		sb.WriteString(" WHERE datname = '")
		sb.WriteString(EscapeSQLString(opts.SourceDB))
		sb.WriteString("'\n")
		sb.WriteString("   AND pid <> pg_backend_pid();\n")
	}
	sb.WriteString("CREATE DATABASE ")
	sb.WriteString(QuoteIdentifier(opts.DestDB))
	sb.WriteString(" WITH TEMPLATE ")
	sb.WriteString(QuoteIdentifier(opts.SourceDB))
	sb.WriteString(" STRATEGY ")
	sb.WriteString(strategy)
	sb.WriteString(";\n")
	return sb.String()
}

func QuoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func EscapeSQLString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func quoteArg(value string) string {
	if strings.ContainsAny(value, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return value
}
