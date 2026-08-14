package sty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupLocalSSHTest(t *testing.T) localSSHUser {
	t.Helper()
	origUser, origProbe := bootSSHUser, bootSSHProbe
	origGenerate, origDerive := bootSSHGenerate, bootSSHDerive
	t.Cleanup(func() {
		bootSSHUser, bootSSHProbe = origUser, origProbe
		bootSSHGenerate, bootSSHDerive = origGenerate, origDerive
	})
	identity := localSSHUser{Name: "admin", Home: t.TempDir(), UID: os.Getuid(), GID: os.Getgid()}
	bootSSHUser = func() (localSSHUser, error) { return identity, nil }
	return identity
}

func TestEnsureLocalSSHAuthorizesExistingKey(t *testing.T) {
	identity := setupLocalSSHTest(t)
	sshDir := filepath.Join(identity.Home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	privateKey := filepath.Join(sshDir, "id_ed25519")
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFixture admin@test\n"
	if err := os.WriteFile(privateKey, []byte("private fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privateKey+".pub", []byte(publicKey), 0o644); err != nil {
		t.Fatal(err)
	}
	probes := 0
	bootSSHProbe = func(_ context.Context, user localSSHUser, keys []string) error {
		probes++
		if user != identity {
			t.Fatalf("SSH user = %+v", user)
		}
		if probes == 1 {
			return errors.New("not authorized")
		}
		if len(keys) != 1 || keys[0] != privateKey {
			t.Fatalf("repair probe keys = %v", keys)
		}
		return nil
	}
	bootSSHGenerate = func(context.Context, string) error {
		t.Fatal("existing key should not be replaced")
		return nil
	}
	bootSSHDerive = func(context.Context, string) (string, error) {
		t.Fatal("existing public key should not be derived again")
		return "", nil
	}

	status, err := ensureLocalSSH(context.Background())
	if err != nil || status != "repaired" {
		t.Fatalf("SSH repair = %q, %v", status, err)
	}
	authorized, err := os.ReadFile(filepath.Join(sshDir, "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(authorized), "AAAAC3NzaC1lZDI1NTE5AAAAIFixture") != 1 {
		t.Fatalf("authorized_keys = %q", authorized)
	}
	if info, err := os.Stat(privateKey); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("existing private key mode = %v, %v", info, err)
	}
}

func TestEnsureLocalSSHGeneratesKeyWhenMissing(t *testing.T) {
	identity := setupLocalSSHTest(t)
	probes := 0
	bootSSHProbe = func(_ context.Context, _ localSSHUser, keys []string) error {
		probes++
		if probes == 1 {
			return errors.New("no key")
		}
		if len(keys) != 1 || filepath.Base(keys[0]) != "id_rsa" {
			t.Fatalf("generated repair key = %v", keys)
		}
		return nil
	}
	bootSSHGenerate = func(_ context.Context, privateKey string) error {
		if err := os.WriteFile(privateKey, []byte("private fixture"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(privateKey+".pub", []byte("ssh-rsa AAAAB3NzaFixture generated@test\n"), 0o644)
	}
	bootSSHDerive = func(context.Context, string) (string, error) {
		t.Fatal("generated key already has a public key")
		return "", nil
	}

	status, err := ensureLocalSSH(context.Background())
	if err != nil || status != "repaired" {
		t.Fatalf("SSH repair = %q, %v", status, err)
	}
	for _, path := range []string{
		filepath.Join(identity.Home, ".ssh", "id_rsa"),
		filepath.Join(identity.Home, ".ssh", "authorized_keys"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestAuthorizeLocalSSHKeyIsIdempotent(t *testing.T) {
	identity := localSSHUser{Name: "admin", Home: t.TempDir(), UID: os.Getuid(), GID: os.Getgid()}
	path := filepath.Join(identity.Home, "authorized_keys")
	key := []byte("ssh-ed25519 AAAAC3Fixture comment\n")
	if err := authorizeLocalSSHKey(path, key, identity); err != nil {
		t.Fatal(err)
	}
	if err := authorizeLocalSSHKey(path, key, identity); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "AAAAC3Fixture") != 1 {
		t.Fatalf("duplicate key written: %q", content)
	}
}
