package sty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"pig/internal/config"
	"strings"
	"testing"
)

func setupDefaultPigstyTest(t *testing.T) localSSHUser {
	t.Helper()
	origUser, origNetwork, origRun := bootInitUser, bootPigstySourceAvailable, bootRunPigstyInit
	t.Cleanup(func() {
		bootInitUser, bootPigstySourceAvailable, bootRunPigstyInit = origUser, origNetwork, origRun
	})
	identity := localSSHUser{Name: "admin", Home: t.TempDir(), UID: os.Getuid(), GID: os.Getgid()}
	bootInitUser = func() (localSSHUser, error) { return identity, nil }
	return identity
}

func TestEnsureDefaultPigstyPreservesExistingPath(t *testing.T) {
	identity := setupDefaultPigstyTest(t)
	target := filepath.Join(identity.Home, "pigsty")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	bootPigstySourceAvailable = func() bool {
		t.Fatal("existing Pigsty path triggered a network probe")
		return false
	}
	bootRunPigstyInit = func(context.Context, localSSHUser, string, string) error {
		t.Fatal("existing Pigsty path was reinitialized")
		return nil
	}

	outcome, err := ensureDefaultPigsty(context.Background(), "")
	if err != nil || outcome.Path != target || outcome.Status != pigstyInitExisting || outcome.Warning != "" {
		t.Fatalf("existing outcome = %+v, err=%v", outcome, err)
	}
}

func TestEnsureDefaultPigstySkipsWhenSourceUnavailable(t *testing.T) {
	identity := setupDefaultPigstyTest(t)
	bootPigstySourceAvailable = func() bool { return false }
	bootRunPigstyInit = func(context.Context, localSSHUser, string, string) error {
		t.Fatal("unreachable source triggered pig sty init")
		return nil
	}

	outcome, err := ensureDefaultPigsty(context.Background(), "")
	if err != nil || outcome.Path != filepath.Join(identity.Home, "pigsty") || outcome.Status != pigstyInitUnavailable {
		t.Fatalf("unavailable outcome = %+v, err=%v", outcome, err)
	}
	if !strings.Contains(outcome.Warning, "run pig sty init") {
		t.Fatalf("unavailable warning = %q", outcome.Warning)
	}
}

func TestEnsureDefaultPigstyRunsInitAsInvokingUser(t *testing.T) {
	identity := setupDefaultPigstyTest(t)
	bootPigstySourceAvailable = func() bool { return true }
	calls := 0
	bootRunPigstyInit = func(_ context.Context, gotUser localSSHUser, target, region string) error {
		calls++
		if gotUser != identity || target != filepath.Join(identity.Home, "pigsty") || region != "china" {
			t.Fatalf("init args = %+v %q %q", gotUser, target, region)
		}
		return nil
	}

	outcome, err := ensureDefaultPigsty(context.Background(), "china")
	if err != nil || outcome.Status != pigstyInitInitialized || calls != 1 {
		t.Fatalf("initialized outcome = %+v, calls=%d err=%v", outcome, calls, err)
	}
}

func TestEnsureDefaultPigstyReturnsInitFailure(t *testing.T) {
	setupDefaultPigstyTest(t)
	bootPigstySourceAvailable = func() bool { return true }
	bootRunPigstyInit = func(context.Context, localSSHUser, string, string) error {
		return errors.New("download failed")
	}

	outcome, err := ensureDefaultPigsty(context.Background(), "")
	if err == nil || outcome.Status != pigstyInitFailed || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("failed outcome = %+v, err=%v", outcome, err)
	}
}

func TestBootstrapUserEnvironmentUsesInvokingAccount(t *testing.T) {
	t.Setenv("HOME", "/root")
	t.Setenv("USER", "root")
	t.Setenv("LOGNAME", "root")
	t.Setenv("SUDO_USER", "admin")
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "1000")
	t.Setenv("PIG_TEST_PRESERVE", "yes")
	identity := localSSHUser{Name: "admin", Home: "/home/admin", UID: 1000, GID: 1000}
	env := bootstrapUserEnvironment(identity)
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{"\nHOME=/home/admin\n", "\nUSER=admin\n", "\nLOGNAME=admin\n", "\nPIG_TEST_PRESERVE=yes\n"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("environment missing %q: %v", want, env)
		}
	}
	for _, forbidden := range []string{"\nHOME=/root\n", "\nUSER=root\n", "\nSUDO_USER=", "\nSUDO_UID=", "\nSUDO_GID="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("environment retained %q: %v", forbidden, env)
		}
	}
}

func TestRunDefaultPigstyInitReusesCLIAsInvokingUser(t *testing.T) {
	origExecutable := bootExecutable
	origFormat := config.OutputFormat
	t.Cleanup(func() {
		bootExecutable = origExecutable
		config.OutputFormat = origFormat
	})
	config.OutputFormat = config.OUTPUT_TEXT

	root := t.TempDir()
	capture := filepath.Join(root, "capture")
	script := filepath.Join(root, "pig")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$PIG_INIT_CAPTURE\"\n" +
		"printf '%s\\n' \"$HOME|$USER|$LOGNAME|${SUDO_UID-unset}\" >> \"$PIG_INIT_CAPTURE\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	bootExecutable = func() (string, error) { return script, nil }
	t.Setenv("PIG_INIT_CAPTURE", capture)
	t.Setenv("SUDO_UID", "1234")
	identity := localSSHUser{Name: "admin", Home: root, UID: os.Geteuid(), GID: os.Getegid()}
	target := filepath.Join(root, "pigsty")

	if err := runDefaultPigstyInit(context.Background(), identity, target, "china"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 8 {
		t.Fatalf("captured init invocation = %q", string(data))
	}
	want := []string{"sty", "init", "--path", target, "--dir"}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("init argv[%d] = %q, want %q; all=%v", i, lines[i], want[i], lines)
		}
	}
	downloadDir := lines[5]
	if lines[6] != "--mirror" || lines[7] != root+"|admin|admin|unset" {
		t.Fatalf("init mirror/environment = %v", lines)
	}
	if _, err := os.Stat(downloadDir); !os.IsNotExist(err) {
		t.Fatalf("temporary download directory was not removed: %s err=%v", downloadDir, err)
	}
}
