package ext

import (
	"os"
	"path/filepath"
	"pig/internal/config"
	"reflect"
	"strings"
	"testing"
)

func TestParseAptDependsOutput(t *testing.T) {
	input := `
Depends: libc6
Depends: libssl3
 |Depends: libfoo1
PreDepends: dpkg
Depends: <debconf-2.0>
 |Depends: <virtual-package>
Suggests: foo-doc
<none>
`
	got := parseAptDependsOutput(input)
	want := []string{"libc6", "libssl3", "libfoo1", "dpkg", "foo-doc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseAptDependsOutput() = %#v, want %#v", got, want)
	}
}

func TestDownloadDEBSkipsVirtualDependencies(t *testing.T) {
	bin, dir := t.TempDir(), t.TempDir()
	oldUser := config.CurrentUser
	t.Cleanup(func() { config.CurrentUser = oldUser })
	config.CurrentUser = "root"
	for name, script := range map[string]string{
		"apt-cache": "printf 'Depends: libc6\\nDepends: <debconf-2.0>\\n |Depends: <virtual>\\n'\n",
		"apt-get":   "printf '%s\\n' \"$@\" > args\nfor arg do case \"$arg\" in \\<*) exit 42;; esac; done\ntouch downloaded.deb\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := DownloadDEB([]string{"fixture"}, dir)
	if err != nil || !reflect.DeepEqual(got, []string{"fixture", "libc6"}) {
		t.Fatalf("download result=%v err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "downloaded.deb")); err != nil {
		t.Fatal(err)
	}
}

func TestMergeDEBCandidates(t *testing.T) {
	pkgs := []string{"pkgA", "pkgB", "pkgA"}
	deps := map[string][]string{
		"pkgA": {"dep1", "dep2", "dep1"},
		"pkgB": {"dep2", "dep3"},
	}

	got := mergeDEBCandidates(pkgs, deps)
	want := []string{"pkgA", "pkgB", "dep1", "dep2", "dep3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeDEBCandidates() = %#v, want %#v", got, want)
	}
}

func TestRunCommandInDirDoesNotChangeProcessWorkingDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	workDir := t.TempDir()
	if err := runCommandInDir([]string{"sh", "-c", "touch marker.file"}, workDir); err != nil {
		t.Fatalf("runCommandInDir failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "marker.file")); err != nil {
		t.Fatalf("expected marker file in workDir, got error: %v", err)
	}

	cwdAfter, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd after run failed: %v", err)
	}
	if cwdAfter != cwd {
		t.Fatalf("process working directory changed: before=%s after=%s", cwd, cwdAfter)
	}
}

func TestSudoCommandInDirNonInteractive(t *testing.T) {
	oldUser, oldFormat, oldInput := config.CurrentUser, config.OutputFormat, os.Stdin
	t.Cleanup(func() { config.CurrentUser, config.OutputFormat, os.Stdin = oldUser, oldFormat, oldInput })
	config.CurrentUser = "caller"
	bin := t.TempDir()
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PIG_TEST_EXT_ARGS\"\n[ \"$1\" != -n ] || shift\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIG_NO_SUDO", "")
	t.Setenv("PIG_TEST_EXT_ARGS", capture)
	for _, mode := range []string{"text", "env", "json", "yaml"} {
		t.Run(mode, func(t *testing.T) {
			config.OutputFormat = config.OUTPUT_TEXT
			t.Setenv("PIG_NON_INTERACTIVE", "")
			if mode == "env" {
				t.Setenv("PIG_NON_INTERACTIVE", "1")
			}
			if mode == "json" || mode == "yaml" {
				config.OutputFormat = mode
			}
			in, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			_, _ = in.WriteString("sentinel\n")
			_, _ = in.Seek(0, 0)
			os.Stdin = in
			if err := sudoCommandInDir([]string{"sh", "-c", `pwd -P > cwd; if read -r line; then printf '%s' "$line" > input; else : > input; fi`}, dir); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(filepath.Join(dir, "input"))
			if (string(got) == "sentinel") != (mode == "text") {
				t.Errorf("input=%q mode=%s", got, mode)
			}
			cwd, _ := os.ReadFile(filepath.Join(dir, "cwd"))
			resolved, _ := filepath.EvalSymlinks(dir)
			if strings.TrimSpace(string(cwd)) != resolved {
				t.Errorf("cwd=%q", cwd)
			}
			if os.Geteuid() != 0 {
				args, _ := os.ReadFile(capture)
				if strings.HasPrefix(string(args), "-n\n") != (mode != "text") {
					t.Errorf("sudo argv=%q", args)
				}
			}
		})
	}
}
