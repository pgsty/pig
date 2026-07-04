package postgres

import "testing"

func TestBuildPsqlArgsBindsExplicitDataDirToPostmasterInfo(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	dataDir := t.TempDir()
	writeTestPostmasterPid(t, dataDir, "1738656000", "6543", "/tmp/pgsocket")

	args, err := buildPsqlArgs("/usr/bin/psql", &Config{PgData: dataDir, DbSU: dbsu}, "postgres", &PsqlOptions{Command: "select 1"})
	if err != nil {
		t.Fatalf("buildPsqlArgs returned error: %v", err)
	}
	assertArgPair(t, args, "-p", "6543")
	assertArgPair(t, args, "-h", "/tmp/pgsocket")
	assertArgPair(t, args, "-d", "postgres")
	assertArgPair(t, args, "-c", "select 1")
}

func TestBuildPsqlArgsBindsPortWithoutSocketDir(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	dataDir := t.TempDir()
	writeTestPostmasterPid(t, dataDir, "1738656000", "6543", "")

	args, err := buildPsqlArgs("/usr/bin/psql", &Config{PgData: dataDir, DbSU: dbsu}, "", nil)
	if err != nil {
		t.Fatalf("buildPsqlArgs returned error: %v", err)
	}
	assertArgPair(t, args, "-p", "6543")
	assertArgPair(t, args, "-d", "postgres")
	if containsArgValue(args, "-h") {
		t.Fatalf("psql args should omit -h when socket dir is absent, got %v", args)
	}
}

func assertArgPair(t *testing.T, args []string, flag string, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Fatalf("args missing %s %s pair: %v", flag, value, args)
}

func containsArgValue(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}
