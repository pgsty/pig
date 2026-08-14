package sty

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"pig/cli/get"
	"pig/internal/config"
	"pig/internal/utils"
)

const (
	pigstyInitExisting    = "existing"
	pigstyInitInitialized = "initialized"
	pigstyInitLocal       = "initialized-local"
	pigstyInitUnavailable = "unavailable"
	pigstyInitFailed      = "failed"
)

type pigstyInitOutcome struct {
	Path    string
	Status  string
	Warning string
}

type pigstySourceArchive struct {
	Path    string
	Version string
}

var (
	bootInitUser              = bootstrapSSHUser
	bootPigstySourceAvailable = func() bool {
		get.NetworkCondition()
		return get.Source != get.ViaNA
	}
	bootFindLocalPigstySource = findLocalPigstySource
	bootRunPigstyInit         = runDefaultPigstyInit
	bootRunLocalPigstyInit    = runLocalPigstyInit
	bootExecutable            = os.Executable
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

	if bootPigstySourceAvailable() {
		if err := bootRunPigstyInit(ctx, identity, target, region); err != nil {
			outcome.Status = pigstyInitFailed
			return outcome, err
		}
		outcome.Status = pigstyInitInitialized
		return outcome, nil
	}

	archive, err := bootFindLocalPigstySource(bootRepoDir)
	if err != nil {
		outcome.Status = pigstyInitFailed
		return outcome, fmt.Errorf("inspect local Pigsty source: %w", err)
	}
	if archive == nil {
		outcome.Status = pigstyInitUnavailable
		outcome.Warning = "Pigsty source unavailable: network offline and no local source archive"
		return outcome, nil
	}
	if err := bootRunLocalPigstyInit(ctx, identity, target, *archive); err != nil {
		outcome.Status = pigstyInitFailed
		return outcome, err
	}
	outcome.Status = pigstyInitLocal
	return outcome, nil
}

func findLocalPigstySource(repoDir string) (*pigstySourceArchive, error) {
	entries, err := os.ReadDir(repoDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var selected *pigstySourceArchive
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		version, err := get.GetVerFromName(entry.Name())
		if err != nil {
			continue
		}
		path := filepath.Join(repoDir, entry.Name())
		if !regularFile(path) {
			continue
		}
		if selected == nil || get.CompareVersions(version, selected.Version) > 0 {
			selected = &pigstySourceArchive{Path: path, Version: version}
		}
	}
	return selected, nil
}

// runDefaultPigstyInit reuses the installed CLI surface instead of duplicating
// release selection, download, EULA, extraction, and transactional install
// logic. The child drops root credentials before it creates the target tree.
func runDefaultPigstyInit(ctx context.Context, identity localSSHUser, target, region string) error {
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
	return runPigstyInitCommand(ctx, identity, target, downloadDir, "", region)
}

func runLocalPigstyInit(ctx context.Context, identity localSSHUser, target string, archive pigstySourceArchive) error {
	if !regularFile(archive.Path) || archive.Version == "" {
		return fmt.Errorf("local Pigsty source archive is invalid: %s", archive.Path)
	}
	return runPigstyInitCommand(ctx, identity, target, filepath.Dir(archive.Path), archive.Version, "")
}

func runPigstyInitCommand(ctx context.Context, identity localSSHUser, target, downloadDir, version, region string) error {
	executable, err := bootExecutable()
	if err != nil {
		return fmt.Errorf("resolve pig executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve pig executable symlinks: %w", err)
	}

	args := []string{"sty", "init", "--path", target, "--dir", downloadDir}
	if version != "" {
		args = append(args, "--version", version)
	}
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

	stdout := utils.NewTailBuffer(64 * 1024)
	stderr := utils.NewTailBuffer(64 * 1024)
	var childStdout io.Writer = stdout
	if !config.IsStructuredOutput() && bootProgressOut != nil {
		childStdout = io.MultiWriter(bootProgressOut, stdout)
	}
	cmd.Stdout = childStdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if len(detail) > 2048 {
			cut := len(detail) - 2048
			for cut < len(detail) && !utf8.RuneStart(detail[cut]) {
				cut++
			}
			detail = detail[cut:]
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
		case "HOME", "USER", "LOGNAME", "PWD", "SUDO_USER", "SUDO_UID", "SUDO_GID", "PIG_NON_INTERACTIVE":
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"HOME="+identity.Home,
		"USER="+identity.Name,
		"LOGNAME="+identity.Name,
	)
	// The child pig sty init may legitimately prompt (e.g. Pro EULA
	// acceptance) and its stdin is wired to the terminal; only force
	// non-interactive mode when no terminal conversation is possible.
	if config.IsStructuredOutput() || !bootStdinIsTerminal() {
		env = append(env, "PIG_NON_INTERACTIVE=1")
	}
	return env
}

func cleanupPigstyInitTemp(path string) {
	path = filepath.Clean(path)
	if filepath.Dir(path) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(path), "pigsty-init-") {
		return
	}
	_ = os.RemoveAll(path)
}
