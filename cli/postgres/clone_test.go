package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
)

func TestBuildDatabaseCloneSQL(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&CloneOptions{
		SourceDB: "app",
		DestDB:   "app_fork",
		Owner:    "app_owner",
	})

	for _, want := range []string{
		"\\set ON_ERROR_STOP on",
		"SELECT pg_terminate_backend(pid)",
		`datname = 'app'`,
		`CREATE DATABASE "app_fork" WITH TEMPLATE "app" STRATEGY FILE_COPY;`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "OWNER") {
		t.Fatalf("clone SQL should not include OWNER clause:\n%s", sql)
	}
}

func TestBuildDatabaseAlterOwnerSQL(t *testing.T) {
	sql := BuildDatabaseAlterOwnerSQL("app_fork", "app_owner")
	want := "\\set ON_ERROR_STOP on\n" + `ALTER DATABASE "app_fork" OWNER TO "app_owner";` + "\n"
	if sql != want {
		t.Fatalf("BuildDatabaseAlterOwnerSQL() = %q", sql)
	}
}

func TestBuildDatabaseAlterOwnerSQLEmptyOwner(t *testing.T) {
	if sql := BuildDatabaseAlterOwnerSQL("app_fork", ""); sql != "" {
		t.Fatalf("BuildDatabaseAlterOwnerSQL empty owner = %q, want empty", sql)
	}
}

func TestBuildDatabaseCloneSQLAlwaysTerminatesSourceConnections(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&CloneOptions{
		SourceDB: "app",
		DestDB:   "app_fork",
	})
	if !strings.Contains(sql, "pg_terminate_backend") {
		t.Fatalf("SQL should terminate source connections:\n%s", sql)
	}
}

func TestBuildDatabaseCloneSQLIncludesConnectionLimitWhenSet(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&CloneOptions{
		SourceDB:     "app",
		DestDB:       "app_fork",
		ConnLimit:    12,
		ConnLimitSet: true,
	})
	if !strings.Contains(sql, `CONNECTION LIMIT 12`) {
		t.Fatalf("SQL should include connection limit:\n%s", sql)
	}
}

func TestBuildDatabaseCloneSQLAllowsZeroConnectionLimit(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&CloneOptions{
		SourceDB:     "app",
		DestDB:       "app_fork",
		ConnLimit:    0,
		ConnLimitSet: true,
	})
	if !strings.Contains(sql, `CONNECTION LIMIT 0`) {
		t.Fatalf("SQL should include zero connection limit:\n%s", sql)
	}
}

func TestBuildDatabaseCloneSQLOmitsStrategyBeforePG15(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&CloneOptions{
		SourceDB:  "app",
		DestDB:    "app_fork",
		Preflight: ClonePreflight{ServerVersion: 140000},
	})
	if strings.Contains(sql, "STRATEGY") {
		t.Fatalf("PG14 clone SQL should use default template copy without STRATEGY:\n%s", sql)
	}
}

func TestBuildDatabaseCloneSQLIncludesStrategyFromPG15(t *testing.T) {
	sql := BuildDatabaseCloneSQL(&CloneOptions{
		SourceDB:  "app",
		DestDB:    "app_fork",
		Preflight: ClonePreflight{ServerVersion: 150000},
	})
	if !strings.Contains(sql, "STRATEGY FILE_COPY") {
		t.Fatalf("PG15+ clone SQL should include STRATEGY FILE_COPY:\n%s", sql)
	}
}

func TestNextDatabaseCloneNameUsesFirstAvailableSuffix(t *testing.T) {
	names := map[string]bool{
		"app":   true,
		"app_1": true,
		"app_2": true,
	}
	if got := NextDatabaseCloneName("app", names); got != "app_3" {
		t.Fatalf("NextDatabaseCloneName() = %q, want app_3", got)
	}
}

func TestNextDatabaseCloneNameKeepsGeneratedNameWithinPostgresLimit(t *testing.T) {
	const postgresIdentifierLimit = 63
	source := strings.Repeat("a", 63)
	names := map[string]bool{source: true}

	got := NextDatabaseCloneName(source, names)
	if len(got) > postgresIdentifierLimit {
		t.Fatalf("NextDatabaseCloneName() length = %d, want <= %d: %q", len(got), postgresIdentifierLimit, got)
	}
	if !strings.HasSuffix(got, "_1") {
		t.Fatalf("NextDatabaseCloneName() = %q, want _1 suffix", got)
	}
	if got == source {
		t.Fatalf("NextDatabaseCloneName() should not return source name %q", source)
	}
}

func TestParseDatabaseNamesOneNamePerLine(t *testing.T) {
	names := parseDatabaseNames("app\napp_1\npostgres\n")
	if len(names) != 3 || names[0] != "app" || names[1] != "app_1" || names[2] != "postgres" {
		t.Fatalf("parseDatabaseNames returned %#v", names)
	}
}

func TestClonePreflightWarnings(t *testing.T) {
	checks := ClonePreflight{
		ServerVersion: 170000,
		DataDirectory: "/pg/data",
		FileSystem:    "ext4",
		CloneMode:     DatabaseCloneModeCopy,
		Strategy:      "FILE_COPY",
	}
	warnings := checks.Warnings()
	for _, want := range []string{
		"PostgreSQL 18+",
		"regular database copy",
	} {
		if !containsCloneString(warnings, want) {
			t.Fatalf("warnings %#v should contain %q", warnings, want)
		}
	}
	if containsCloneString(warnings, "file_copy_method=clone could not be verified") {
		t.Fatalf("PG15-17 warnings should not report file_copy_method as a query failure: %#v", warnings)
	}
}

func TestClonePreflightWarningsIncludePG14DefaultCopy(t *testing.T) {
	checks := ClonePreflight{
		ServerVersion: 140000,
		Strategy:      "DEFAULT",
		CloneMode:     DatabaseCloneModeCopy,
	}
	warnings := checks.Warnings()
	for _, want := range []string{
		"PostgreSQL 15+",
		"default template copy",
		"PostgreSQL 18+",
	} {
		if !containsCloneString(warnings, want) {
			t.Fatalf("warnings %#v should contain %q", warnings, want)
		}
	}
}

func TestClonePreflightWarningsIncludeFileCopyMethodError(t *testing.T) {
	checks := ClonePreflight{
		ServerVersion:       180000,
		FileCopyMethodError: `ERROR: unrecognized configuration parameter "file_copy_method"`,
		DataDirectory:       "/pg/data",
		FileSystem:          "xfs",
		CloneMode:           DatabaseCloneModeCOW,
		Strategy:            "FILE_COPY",
	}
	warnings := checks.Warnings()
	if !containsCloneString(warnings, "unrecognized configuration parameter") {
		t.Fatalf("warnings %#v should include file_copy_method query error", warnings)
	}
}

func TestClonePreflightWarningsDoNotWarnOnStrategyField(t *testing.T) {
	checks := ClonePreflight{
		ServerVersion:  180000,
		FileCopyMethod: "clone",
		DataDirectory:  "/pg/data",
		FileSystem:     "xfs",
		CloneMode:      DatabaseCloneModeCOW,
		Strategy:       "WAL_LOG",
	}
	if warnings := checks.Warnings(); containsCloneString(warnings, "strategy") {
		t.Fatalf("strategy field should not create warnings: %#v", warnings)
	}
}

func TestClonePreflightWarningsPassForPG18CloneCOW(t *testing.T) {
	checks := ClonePreflight{
		ServerVersion:  180000,
		FileCopyMethod: "clone",
		DataDirectory:  "/pg/data",
		FileSystem:     "xfs",
		CloneMode:      DatabaseCloneModeCOW,
		Strategy:       "FILE_COPY",
	}
	if warnings := checks.Warnings(); len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
}

func TestNormalizeCloneAllowsGeneratedDestination(t *testing.T) {
	n, err := NormalizeCloneOptions(&CloneOptions{
		SourceDB: "app",
	})
	if err != nil {
		t.Fatalf("NormalizeCloneOptions returned error: %v", err)
	}
	if n.DestDB != "" {
		t.Fatalf("DestDB = %q, want empty before database name resolution", n.DestDB)
	}
}

func TestNormalizeCloneRejectsTemplateSources(t *testing.T) {
	for _, source := range []string{"template0", "template1"} {
		_, err := NormalizeCloneOptions(&CloneOptions{
			SourceDB: source,
			DestDB:   source + "_1",
		})
		if err == nil {
			t.Fatalf("NormalizeCloneOptions should reject source %s", source)
		}
	}
}

func TestNormalizeCloneRejectsConnDBMatchingSource(t *testing.T) {
	_, err := NormalizeCloneOptions(&CloneOptions{
		SourceDB: "app",
		DestDB:   "app_1",
		ConnDB:   "app",
	})
	if err == nil {
		t.Fatal("NormalizeCloneOptions should reject conn db matching source db")
	}
}

func TestNormalizeCloneUsesTemplate1WhenCloningPostgres(t *testing.T) {
	n, err := NormalizeCloneOptions(&CloneOptions{
		SourceDB: "postgres",
		DestDB:   "postgres_1",
	})
	if err != nil {
		t.Fatalf("NormalizeCloneOptions returned error: %v", err)
	}
	if n.ConnDB != "template1" {
		t.Fatalf("ConnDB = %q, want template1", n.ConnDB)
	}
}

func TestNormalizeCloneRejectsOverlongIdentifiers(t *testing.T) {
	const postgresIdentifierLimit = 63
	overlong := strings.Repeat("a", postgresIdentifierLimit+1)
	tests := []CloneOptions{
		{SourceDB: overlong, DestDB: "app_1"},
		{SourceDB: "app", DestDB: overlong},
		{SourceDB: "app", DestDB: "app_1", ConnDB: overlong},
		{SourceDB: "app", DestDB: "app_1", Owner: overlong},
	}

	for _, tt := range tests {
		if _, err := NormalizeCloneOptions(&tt); err == nil {
			t.Fatalf("NormalizeCloneOptions(%+v) should reject overlong identifier", tt)
		}
	}
}

func TestQuoteIdentifierEscapesDoubleQuotes(t *testing.T) {
	got := QuoteIdentifier(`a"b`)
	want := `"a""b"`
	if got != want {
		t.Fatalf("QuoteIdentifier() = %q, want %q", got, want)
	}
}

func TestBuildClonePlan(t *testing.T) {
	opts, err := NormalizeCloneOptions(&CloneOptions{
		SourceDB: "app",
		DestDB:   "app_fork",
		Plan:     true,
	})
	if err != nil {
		t.Fatalf("NormalizeCloneOptions returned error: %v", err)
	}
	opts.Preflight.CloneMode = DatabaseCloneModeCOW

	plan := BuildClonePlan(opts)
	if plan.Command != "pig pg clone app app_fork --plan" {
		t.Errorf("Command = %q, want database clone command", plan.Command)
	}
	for _, want := range []string{
		"Terminate existing connections to app",
		"Create database app_fork from template app",
	} {
		if !containsCloneAction(plan.Actions, want) {
			t.Errorf("plan actions missing %q: %#v", want, plan.Actions)
		}
	}
	if !containsCloneResource(plan.Affects, "database", "app_fork") {
		t.Errorf("plan affects should include destination database app_fork: %#v", plan.Affects)
	}
	if !containsCloneString(plan.Risks, "persistent reconnect") {
		t.Errorf("plan risks should mention persistent reconnect: %#v", plan.Risks)
	}
}

func TestBuildCloneCommandShellQuotesDatabaseNames(t *testing.T) {
	opts := &CloneOptions{
		SourceDB: "app db",
		DestDB:   "app db_1",
		ConnDB:   "postgres maint",
		Owner:    "dba role",
		Plan:     true,
	}

	got := BuildCloneCommand(opts)
	want := "pig pg clone 'app db' 'app db_1' --conn-db 'postgres maint' --owner 'dba role' --plan"
	if got != want {
		t.Fatalf("BuildCloneCommand() = %q, want %q", got, want)
	}
}

func TestCloneResultShellQuotesConnectAndCleanupCommands(t *testing.T) {
	result := cloneResult(&CloneOptions{
		SourceDB: "app",
		DestDB:   "app fork",
		Port:     5432,
	}, 0)

	if result.ConnectCommand != "psql -p 5432 -d 'app fork'" {
		t.Fatalf("ConnectCommand = %q", result.ConnectCommand)
	}
	if result.CleanupCommand != "dropdb -p 5432 'app fork'" {
		t.Fatalf("CleanupCommand = %q", result.CleanupCommand)
	}
}

func TestClonePsqlFileArgsDisablePsqlrc(t *testing.T) {
	args := clonePsqlFileArgs("/usr/bin/psql", 5432, "postgres")
	if len(args) == 0 || args[0] != "/usr/bin/psql" {
		t.Fatalf("clonePsqlFileArgs() = %#v", args)
	}
	if !containsCloneArg(args, "-X") {
		t.Fatalf("clonePsqlFileArgs() should include -X to disable .psqlrc: %#v", args)
	}
	if args[len(args)-1] != "-f" {
		t.Fatalf("clonePsqlFileArgs() should end with -f before SQL path is appended: %#v", args)
	}
}

func TestClonePsqlFileHintShellQuotesConnectionDatabase(t *testing.T) {
	args := clonePsqlFileArgs("/usr/bin/psql", 5432, "postgres maint")
	got := clonePsqlFileHint(args, "<clone-sql>")
	want := "/usr/bin/psql -X -p 5432 -d 'postgres maint' -f <clone-sql>"
	if got != want {
		t.Fatalf("clonePsqlFileHint() = %q, want %q", got, want)
	}
}

func TestRunPsqlQueryIncludesCapturedOutputOnFailure(t *testing.T) {
	originalUser := config.CurrentUser
	config.CurrentUser = "pigtest"
	t.Cleanup(func() {
		config.CurrentUser = originalUser
	})

	diag := `psql: error: connection to server on socket "/var/run/postgresql/.s.PGSQL.65535" failed: No such file or directory`
	psql := filepath.Join(t.TempDir(), "psql")
	script := fmt.Sprintf("#!/bin/sh\ncat >&2 <<'EOF'\n%s\nEOF\nexit 2\n", diag)
	if err := os.WriteFile(psql, []byte(script), 0755); err != nil {
		t.Fatalf("write fake psql: %v", err)
	}

	_, err := runPsqlQuery("pigtest", psql, 65535, "postgres", "SELECT 1")
	if err == nil {
		t.Fatal("runPsqlQuery should fail when psql exits non-zero")
	}
	if !strings.Contains(err.Error(), diag) {
		t.Fatalf("error %q should include captured psql output %q", err.Error(), diag)
	}
}

func containsCloneAction(actions []output.Action, text string) bool {
	for _, action := range actions {
		if strings.Contains(action.Description, text) {
			return true
		}
	}
	return false
}

func containsCloneResource(resources []output.Resource, typ, name string) bool {
	for _, resource := range resources {
		if resource.Type == typ && resource.Name == name {
			return true
		}
	}
	return false
}

func containsCloneArg(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsCloneString(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
