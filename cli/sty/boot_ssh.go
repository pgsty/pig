package sty

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type localSSHUser struct {
	Name string
	Home string
	UID  int
	GID  int
}

var (
	bootSSHUser     = bootstrapSSHUser
	bootSSHProbe    = probeLocalSSH
	bootSSHGenerate = generateLocalSSHKey
	bootSSHDerive   = deriveLocalSSHPublicKey
)

// ensureLocalSSH mirrors the useful, non-fatal part of the legacy bootstrap:
// use an existing administrator key when possible, otherwise generate one,
// authorize it, and retry localhost. Its caller turns every error into a warning.
func ensureLocalSSH(ctx context.Context) (string, error) {
	identity, err := bootSSHUser()
	if err != nil {
		return "warning", err
	}
	sshDir := filepath.Join(identity.Home, ".ssh")
	privateKeys := localSSHPrivateKeys(sshDir)
	if err := bootSSHProbe(ctx, identity, privateKeys); err == nil {
		return "ok", nil
	}
	if err := prepareLocalSSHDirectory(sshDir, identity); err != nil {
		return "warning", err
	}

	privateKey := ""
	if len(privateKeys) > 0 {
		privateKey = privateKeys[0]
	} else {
		privateKey = filepath.Join(sshDir, "id_rsa")
		if err := bootSSHGenerate(ctx, privateKey); err != nil {
			return "warning", fmt.Errorf("generate localhost SSH key: %w", err)
		}
	}
	if err := setLocalSSHFile(privateKey, 0o600, identity); err != nil {
		return "warning", err
	}

	publicPath := privateKey + ".pub"
	if !regularFile(publicPath) {
		if info, statErr := os.Lstat(publicPath); statErr == nil {
			return "warning", fmt.Errorf("refusing unsafe SSH public key: %s (%s)", publicPath, info.Mode())
		} else if !os.IsNotExist(statErr) {
			return "warning", fmt.Errorf("inspect localhost SSH public key: %w", statErr)
		}
		publicKey, err := bootSSHDerive(ctx, privateKey)
		if err != nil {
			return "warning", fmt.Errorf("derive localhost SSH public key: %w", err)
		}
		f, err := os.OpenFile(publicPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return "warning", fmt.Errorf("create localhost SSH public key: %w", err)
		}
		_, writeErr := f.Write(append([]byte(strings.TrimSpace(publicKey)), '\n'))
		closeErr := f.Close()
		if writeErr != nil {
			return "warning", fmt.Errorf("write localhost SSH public key: %w", writeErr)
		}
		if closeErr != nil {
			return "warning", fmt.Errorf("close localhost SSH public key: %w", closeErr)
		}
	}
	if err := setLocalSSHFile(publicPath, 0o644, identity); err != nil {
		return "warning", err
	}
	publicKey, err := os.ReadFile(publicPath)
	if err != nil {
		return "warning", fmt.Errorf("read localhost SSH public key: %w", err)
	}
	if err := authorizeLocalSSHKey(filepath.Join(sshDir, "authorized_keys"), publicKey, identity); err != nil {
		return "warning", err
	}
	if err := bootSSHProbe(ctx, identity, []string{privateKey}); err != nil {
		return "warning", err
	}
	return "repaired", nil
}

func bootstrapSSHUser() (localSSHUser, error) {
	var account *user.User
	var err error
	if sudoUID := strings.TrimSpace(os.Getenv("SUDO_UID")); sudoUID != "" && sudoUID != "0" {
		account, err = user.LookupId(sudoUID)
		if err != nil {
			return localSSHUser{}, fmt.Errorf("find sudo caller uid %s: %w", sudoUID, err)
		}
	} else {
		account, err = user.Current()
		if err != nil {
			return localSSHUser{}, fmt.Errorf("find bootstrap user: %w", err)
		}
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return localSSHUser{}, fmt.Errorf("parse bootstrap uid %q: %w", account.Uid, err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return localSSHUser{}, fmt.Errorf("parse bootstrap gid %q: %w", account.Gid, err)
	}
	home := filepath.Clean(strings.TrimSpace(account.HomeDir))
	if account.Username == "" || !filepath.IsAbs(home) || home == string(filepath.Separator) {
		return localSSHUser{}, fmt.Errorf("bootstrap user has an invalid name or home directory")
	}
	return localSSHUser{Name: account.Username, Home: home, UID: uid, GID: gid}, nil
}

func localSSHPrivateKeys(sshDir string) []string {
	keys := make([]string, 0, 3)
	for _, name := range []string{"id_rsa", "id_ed25519", "id_ecdsa"} {
		path := filepath.Join(sshDir, name)
		if regularFile(path) {
			keys = append(keys, path)
		}
	}
	return keys
}

func prepareLocalSSHDirectory(path string, identity localSSHUser) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("refusing unsafe SSH directory: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect SSH directory: %w", err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create SSH directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure SSH directory: %w", err)
	}
	return setLocalSSHOwner(path, identity)
}

func setLocalSSHFile(path string, mode os.FileMode, identity localSSHUser) error {
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set SSH file mode for %s: %w", path, err)
	}
	return setLocalSSHOwner(path, identity)
}

func setLocalSSHOwner(path string, identity localSSHUser) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if err := os.Chown(path, identity.UID, identity.GID); err != nil {
		return fmt.Errorf("set SSH ownership for %s: %w", path, err)
	}
	return nil
}

func generateLocalSSHKey(ctx context.Context, privateKey string) error {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return err
	}
	_, err = runControllerProbe(ctx, keygen, "-q", "-t", "rsa", "-b", "2048", "-N", "", "-f", privateKey)
	return err
}

func deriveLocalSSHPublicKey(ctx context.Context, privateKey string) (string, error) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return "", err
	}
	output, err := runControllerProbe(ctx, keygen, "-y", "-f", privateKey)
	return strings.TrimSpace(string(output)), err
}

func authorizeLocalSSHKey(path string, publicKey []byte, identity localSSHUser) error {
	fields := strings.Fields(strings.TrimSpace(string(publicKey)))
	if len(fields) < 2 {
		return fmt.Errorf("invalid localhost SSH public key")
	}
	keyID := fields[0] + " " + fields[1]
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked authorized_keys: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect authorized_keys: %w", err)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read authorized_keys: %w", err)
	}
	found := false
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.Contains(line, keyID) {
			found = true
			break
		}
	}
	if !found {
		prefix := ""
		if len(existing) > 0 && existing[len(existing)-1] != '\n' {
			prefix = "\n"
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open authorized_keys: %w", err)
		}
		_, writeErr := f.WriteString(prefix + strings.TrimSpace(string(publicKey)) + "\n")
		closeErr := f.Close()
		if writeErr != nil {
			return fmt.Errorf("append authorized_keys: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close authorized_keys: %w", closeErr)
		}
	}
	return setLocalSSHFile(path, 0o600, identity)
}

func probeLocalSSH(ctx context.Context, identity localSSHUser, identities []string) error {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	args := []string{
		"-F", "/dev/null", "-T", "-n",
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "NumberOfPasswordPrompts=0",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "UpdateHostKeys=no",
		"-o", "ConnectionAttempts=1",
		"-o", "ConnectTimeout=2",
		"-o", "IdentityFile=none",
		"-o", "IdentitiesOnly=yes",
		"-o", "LogLevel=ERROR",
	}
	for _, identity := range identities {
		args = append(args, "-i", identity)
	}
	args = append(args, identity.Name+"@127.0.0.1", "true")
	cmd := exec.CommandContext(probeCtx, ssh, args...)
	if os.Geteuid() == 0 && identity.UID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
			Uid: uint32(identity.UID),
			Gid: uint32(identity.GID),
		}}
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("ssh: %w", err)
	}
	return fmt.Errorf("ssh: %w: %s", err, detail)
}
