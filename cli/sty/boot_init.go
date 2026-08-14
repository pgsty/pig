package sty

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
)

const (
	pigstyInitExisting    = "existing"
	pigstyInitInitialized = "initialized"
	pigstyInitUnavailable = "unavailable"
	pigstyInitFailed      = "failed"
)

type pigstyInitOutcome struct {
	Path    string
	Status  string
	Warning string
}

var (
	bootInitUser              = bootstrapSSHUser
	bootPigstySourceAvailable = func() bool {
		get.NetworkCondition()
		return get.Source != get.ViaNA
	}
	bootRunPigstyInit = runDefaultPigstyInit
	bootExecutable    = os.Executable
)

// ensureDefaultPigsty initializes ~/pigsty for the invoking administrator,
// not for the root account entered through sudo. An existing path is always
// preserved, and an unavailable Pigsty source is reported without failing an
// otherwise useful offline controller bootstrap.
func ensureDefaultPigsty(ctx context.Context, region string) (pigstyInitOutcome, error) {
	identity, err := bootInitUser()
	if err != nil {
		return pigstyInitOutcome{}, fmt.Errorf("resolve invoking user: %w", err)
	}
	target := filepath.Join(identity.Home, "pigsty")
	outcome := pigstyInitOutcome{Path: target}
	if _, err := os.Lstat(target); err == nil {
		outcome.Status = pigstyInitExisting
		return outcome, nil
	} else if !os.IsNotExist(err) {
		return outcome, fmt.Errorf("inspect default Pigsty path %s: %w", target, err)
	}

	if !bootPigstySourceAvailable() {
		outcome.Status = pigstyInitUnavailable
		outcome.Warning = fmt.Sprintf("default Pigsty path %s is missing and the Pigsty source repository is unreachable; run pig sty init when network access is available", target)
		return outcome, nil
	}

	logrus.Infof("default Pigsty path is missing; running pig sty init as %s", identity.Name)
	if err := bootRunPigstyInit(ctx, identity, target, region); err != nil {
		outcome.Status = pigstyInitFailed
		return outcome, err
	}
	outcome.Status = pigstyInitInitialized
	return outcome, nil
}

// runDefaultPigstyInit reuses the installed CLI surface instead of duplicating
// release selection, download, EULA, extraction, and transactional install
// logic. The child drops root credentials before it creates the target tree.
func runDefaultPigstyInit(ctx context.Context, identity localSSHUser, target, region string) error {
	executable, err := bootExecutable()
	if err != nil {
		return fmt.Errorf("resolve pig executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve pig executable symlinks: %w", err)
	}

	downloadDir, err := os.MkdirTemp("", "pigsty-init-")
	if err != nil {
		return fmt.Errorf("create Pigsty download directory: %w", err)
	}
	defer cleanupPigstyInitTemp(downloadDir)
	if err := os.Chmod(downloadDir, 0o700); err != nil {
		return fmt.Errorf("secure Pigsty download directory: %w", err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(downloadDir, identity.UID, identity.GID); err != nil {
			return fmt.Errorf("assign Pigsty download directory to %s: %w", identity.Name, err)
		}
	}

	args := []string{"sty", "init", "--path", target, "--dir", downloadDir}
	if strings.EqualFold(strings.TrimSpace(region), "china") {
		args = append(args, "--mirror")
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = identity.Home
	cmd.Env = bootstrapUserEnvironment(identity)
	cmd.Stdin = os.Stdin
	if os.Geteuid() == 0 && identity.UID != 0 {
		if identity.UID < 0 || identity.GID < 0 {
			return fmt.Errorf("invalid uid or gid for %s", identity.Name)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
			Uid: uint32(identity.UID),
			Gid: uint32(identity.GID),
		}}
	} else if os.Geteuid() != identity.UID {
		return fmt.Errorf("cannot run pig sty init as uid %d from uid %d", identity.UID, os.Geteuid())
	}

	if !config.IsStructuredOutput() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pig sty init as %s: %w", identity.Name, err)
		}
		return nil
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if len(detail) > 2048 {
			detail = detail[len(detail)-2048:]
		}
		if detail != "" {
			return fmt.Errorf("pig sty init as %s: %w: %s", identity.Name, err, detail)
		}
		return fmt.Errorf("pig sty init as %s: %w", identity.Name, err)
	}
	return nil
}

func bootstrapUserEnvironment(identity localSSHUser) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "HOME", "USER", "LOGNAME", "PWD", "SUDO_USER", "SUDO_UID", "SUDO_GID":
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"HOME="+identity.Home,
		"USER="+identity.Name,
		"LOGNAME="+identity.Name,
	)
}

func cleanupPigstyInitTemp(path string) {
	path = filepath.Clean(path)
	if filepath.Dir(path) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(path), "pigsty-init-") {
		return
	}
	_ = os.RemoveAll(path)
}
