package repo

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"
)

func writeRepoTestCommand(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateRepoPrefersSow(t *testing.T) {
	tests := []struct {
		name       string
		create     func(string) error
		legacyTool string
	}{
		{name: "el", create: CreateRepoEL, legacyTool: "createrepo_c"},
		{name: "deb", create: CreateRepoDEB, legacyTool: "dpkg-scanpackages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			testDir := t.TempDir()
			t.Chdir(testDir)
			repoArg := "-repo with spaces"
			repoDir := filepath.Join(testDir, repoArg)
			if err := os.Mkdir(repoArg, 0o755); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(testDir, "sow-args")
			legacyLog := filepath.Join(testDir, "legacy-called")
			writeRepoTestCommand(t, binDir, "sow", `#!/bin/sh
printf '%s\n' "$@" > "$SOW_TEST_LOG"
repo_dir=
for arg in "$@"; do repo_dir=$arg; done
: > "$repo_dir/repo_complete"
`)
			writeRepoTestCommand(t, binDir, tt.legacyTool, "#!/bin/sh\nexit 0\n")
			writeRepoTestCommand(t, binDir, "sh", `#!/bin/sh
printf 'called\n' > "$LEGACY_TEST_LOG"
: > "$REPO_COMPLETE"
`)
			t.Setenv("PATH", binDir)
			t.Setenv("PIG_NO_SUDO", "1")
			t.Setenv("SOW_TEST_LOG", logPath)
			t.Setenv("LEGACY_TEST_LOG", legacyLog)
			t.Setenv("REPO_COMPLETE", filepath.Join(repoDir, "repo_complete"))

			if err := tt.create(repoArg); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			want := "create\n--pigsty\n--timeout\n10m\n--\n" + repoDir + "\n"
			if string(got) != want {
				t.Fatalf("sow args = %q, want %q", got, want)
			}
			if _, err := os.Stat(legacyLog); !os.IsNotExist(err) {
				t.Fatalf("legacy tool was called although sow exists: %v", err)
			}
		})
	}
}

func TestCreateRepoReportsSowFailureWithoutFallback(t *testing.T) {
	binDir := t.TempDir()
	testDir := t.TempDir()
	legacyLog := filepath.Join(testDir, "legacy-called")
	repoDir := filepath.Join(testDir, "repo")
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoTestCommand(t, binDir, "sow", "#!/bin/sh\nexit 23\n")
	writeRepoTestCommand(t, binDir, "dpkg-scanpackages", "#!/bin/sh\nexit 0\n")
	writeRepoTestCommand(t, binDir, "sh", `#!/bin/sh
printf 'called\n' > "$LEGACY_TEST_LOG"
: > "$REPO_COMPLETE"
`)
	t.Setenv("PATH", binDir)
	t.Setenv("PIG_NO_SUDO", "1")
	t.Setenv("LEGACY_TEST_LOG", legacyLog)
	t.Setenv("REPO_COMPLETE", filepath.Join(repoDir, "repo_complete"))

	err := CreateRepoDEB(repoDir)
	if err == nil || !strings.Contains(err.Error(), "failed to create deb repo with sow") {
		t.Fatalf("error = %v, want sow failure", err)
	}
	if _, err := os.Stat(legacyLog); !os.IsNotExist(err) {
		t.Fatalf("legacy tool was called after sow failed: %v", err)
	}
}

func TestCreateRepoFallsBackToLegacyTools(t *testing.T) {
	tests := []struct {
		name       string
		create     func(string) error
		legacyTool string
		osMajor    int
		scriptWant string
		nonExecSow bool
	}{
		{name: "el", create: CreateRepoEL, legacyTool: "createrepo_c", osMajor: 9, scriptWant: "repo2module -s stable"},
		{name: "el7", create: CreateRepoEL, legacyTool: "createrepo_c", osMajor: 7, scriptWant: "rm -f *.i686.rpm"},
		{name: "deb", create: CreateRepoDEB, legacyTool: "dpkg-scanpackages", scriptWant: "dpkg-scanpackages . /dev/null"},
		{name: "non-executable-sow", create: CreateRepoEL, legacyTool: "createrepo_c", osMajor: 9, scriptWant: "repo2module -s stable", nonExecSow: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldMajor := config.OSMajor
			config.OSMajor = tt.osMajor
			t.Cleanup(func() { config.OSMajor = oldMajor })

			binDir := t.TempDir()
			testDir := t.TempDir()
			t.Chdir(testDir)
			repoArg := "-repo with spaces"
			repoDir := filepath.Join(testDir, repoArg)
			if err := os.Mkdir(repoArg, 0o755); err != nil {
				t.Fatal(err)
			}
			scriptLog := filepath.Join(testDir, "legacy-script")
			writeRepoTestCommand(t, binDir, tt.legacyTool, "#!/bin/sh\nexit 0\n")
			if tt.nonExecSow {
				sowPath := writeRepoTestCommand(t, binDir, "sow", "#!/bin/sh\nexit 99\n")
				if err := os.Chmod(sowPath, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			writeRepoTestCommand(t, binDir, "sh", `#!/bin/sh
while IFS= read -r line || [ -n "$line" ]; do
  printf '%s\n' "$line"
done < "$1" > "$SCRIPT_TEST_LOG"
: > "$REPO_COMPLETE"
`)
			t.Setenv("PATH", binDir)
			t.Setenv("PIG_NO_SUDO", "1")
			t.Setenv("SCRIPT_TEST_LOG", scriptLog)
			t.Setenv("REPO_COMPLETE", filepath.Join(repoDir, "repo_complete"))

			if err := tt.create(repoArg); err != nil {
				t.Fatal(err)
			}
			script, err := os.ReadFile(scriptLog)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(script), tt.scriptWant) {
				t.Fatalf("legacy script does not contain %q:\n%s", tt.scriptWant, script)
			}
		})
	}
}

func TestCreateRepoFailsWithoutAnyBackend(t *testing.T) {
	tests := []struct {
		name   string
		create func(string) error
		want   string
	}{
		{name: "el", create: CreateRepoEL, want: "createrepo_c"},
		{name: "deb", create: CreateRepoDEB, want: "dpkg-scanpackages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			t.Setenv("PIG_NO_SUDO", "1")

			err := tt.create(t.TempDir())
			if err == nil {
				t.Fatal("expected missing repository backend error")
			}
			for _, want := range []string{"sow", tt.want} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err, want)
				}
			}
		})
	}
}

func TestCreateRepoRejectsInvalidCompletionMarker(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
		want  string
	}{
		{name: "missing", want: "without completion marker"},
		{name: "directory", setup: func(t *testing.T, marker string) {
			if err := os.Mkdir(marker, 0o755); err != nil {
				t.Fatal(err)
			}
		}, want: "not a regular file"},
		{name: "symlink", setup: func(t *testing.T, marker string) {
			target := filepath.Join(filepath.Dir(marker), "marker-target")
			if err := os.WriteFile(target, []byte("complete\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, marker); err != nil {
				t.Fatal(err)
			}
		}, want: "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			testDir := t.TempDir()
			repoDir := filepath.Join(testDir, "repo")
			if err := os.Mkdir(repoDir, 0o755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(repoDir, "repo_complete")
			if tt.setup != nil {
				tt.setup(t, marker)
			}
			legacyLog := filepath.Join(testDir, "legacy-called")
			writeRepoTestCommand(t, binDir, "sow", "#!/bin/sh\nexit 0\n")
			writeRepoTestCommand(t, binDir, "createrepo_c", "#!/bin/sh\nexit 0\n")
			writeRepoTestCommand(t, binDir, "sh", `#!/bin/sh
printf 'called\n' > "$LEGACY_TEST_LOG"
exit 42
`)
			t.Setenv("PATH", binDir)
			t.Setenv("PIG_NO_SUDO", "1")
			t.Setenv("LEGACY_TEST_LOG", legacyLog)

			err := CreateRepoEL(repoDir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if _, err := os.Stat(legacyLog); !os.IsNotExist(err) {
				t.Fatalf("legacy tool was called after invalid sow marker: %v", err)
			}
		})
	}
}
