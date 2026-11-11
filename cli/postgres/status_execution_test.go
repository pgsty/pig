package postgres

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

func TestTextStatusSharesStructuredExitCodes(t *testing.T) {
	dbsu := withCurrentUserAsDBSU(t)
	installFakePostgresCommands(t, map[string]string{
		"psql":           "printf 'f\\n'\n",
		"pg_ctl":         "printf 'native pg_ctl status diagnostic\\n'; exit 3\n",
		"pg_controldata": "printf 'Database cluster state: shut down\\n'\n",
		"ps":             "printf 'PID PPID START COMMAND\\n'\n",
		"systemctl":      "printf 'inactive\\n'; exit 3\n",
	})
	oldFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = oldFormat })
	config.OutputFormat = config.OUTPUT_TEXT
	for _, tc := range []struct {
		name string
		code int
	}{
		{"running", 0},
		{"stopped", output.CodePgStatusNotRunning},
		{"missing-data", output.CodePgStatusDataDirNotFound},
		{"uninitialized", output.CodePgStatusNotInitialized},
		{"invalid-pid", output.CodePgStatusProbeFailed},
		{"identity-failure", output.CodePgStatusProbeFailed},
		{"permission-denied", output.CodePgStatusPermissionDenied},
		{"no-install-running", 0},
		{"no-install-stopped", output.CodePgStatusNotRunning},
		{"no-install-missing-data", output.CodePgStatusDataDirNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := runtimeFixture(t, dbsu, os.Getpid(), "")
			mockPostmasterIdentity(t)
			if strings.HasPrefix(tc.name, "no-install-") {
				cfg.PgVersion = 999
			}
			switch {
			case strings.HasSuffix(tc.name, "stopped"):
				if err := os.Remove(filepath.Join(cfg.PgData, "postmaster.pid")); err != nil {
					t.Fatal(err)
				}
			case strings.HasSuffix(tc.name, "missing-data"):
				cfg.PgData = filepath.Join(cfg.PgData, "absent")
			case tc.name == "uninitialized":
				if err := os.Remove(filepath.Join(cfg.PgData, "PG_VERSION")); err != nil {
					t.Fatal(err)
				}
			case tc.name == "invalid-pid":
				if err := os.WriteFile(filepath.Join(cfg.PgData, "postmaster.pid"), []byte("invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case tc.name == "identity-failure" || tc.name == "permission-denied":
				previous := postgresDBSUCommandOutput
				t.Cleanup(func() { postgresDBSUCommandOutput = previous })
				postgresDBSUCommandOutput = func(user string, args []string) (string, error) {
					if args[0] == "ps" {
						if tc.name == "permission-denied" {
							return "", syscall.EPERM
						}
						return "", errors.New("identity unavailable")
					}
					return previous(user, args)
				}
			}
			result := StatusResult(cfg)
			if result.Code != tc.code {
				t.Fatalf("structured status code = %d, want %d", result.Code, tc.code)
			}
			data, ok := result.Data.(*PgStatusResultData)
			if !ok || data == nil || data.DataDir != cfg.PgData {
				t.Fatalf("status DTO absent or wrong on %s: %#v", tc.name, result.Data)
			}
			var err error
			text := captureCtlStdout(t, func() { err = Status(cfg) })
			if got := utils.ExitCode(err); got != result.ExitCode() {
				t.Fatalf("text exit = %d (%v), structured = %d; text=%s", got, err, result.ExitCode(), text)
			}
			if err != nil && !utils.IsSilentExit(err) {
				t.Fatalf("text status returned non-silent error %#v; Cobra/root logger would emit Error/Usage chatter", err)
			}
			for _, section := range []string{"[pg_ctl status]", "[PostgreSQL Processes]", "[Related Services]"} {
				if !strings.Contains(text, section) {
					t.Errorf("text status lost %s: %s", section, text)
				}
			}
			if tc.code != 0 && tc.code != output.CodePgStatusNotRunning {
				if !strings.Contains(text, "unknown") || strings.Contains(text, "DOWN") {
					t.Fatalf("failed probe reported a stopped server: %s", text)
				}
				for _, line := range strings.Split(text, "\n") {
					if strings.Contains(line, "postgres:") && !strings.Contains(line, "unknown") {
						t.Fatalf("selected instance service row lost unknown state: %s", line)
					}
				}
			}
		})
	}
}
