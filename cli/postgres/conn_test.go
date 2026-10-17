package postgres

import "testing"

func TestBuildPsqlArgsBindsExplicitDataDirToPostmasterInfo(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
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

func TestBuildPsqlArgsRejectsMissingSocket(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	mockPostmasterIdentity(t)
	dataDir := t.TempDir()
	writeTestPostmasterPid(t, dataDir, "1738656000", "6543", "")
	if _, err := buildPsqlArgs("/usr/bin/psql", &Config{PgData: dataDir, DbSU: dbsu}, "", nil); err == nil {
		t.Fatal("explicit target without socket must not fall back to libpq defaults")
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
