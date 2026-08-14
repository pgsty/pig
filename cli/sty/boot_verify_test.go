package sty

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnsiblePythonExecutableUsesVersionRuntime(t *testing.T) {
	output := []byte("ansible-playbook [core 2.20.1]\n  python version = 3.14.3 (main) [GCC] (/opt/ansible/bin/python)\n")
	got, err := ansiblePythonExecutable("/missing/ansible-playbook", output)
	if err != nil || got != "/opt/ansible/bin/python" {
		t.Fatalf("Ansible Python = %q, %v", got, err)
	}
}

func TestAnsiblePythonExecutableFallsBackToShebang(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ansible-playbook")
	if err := os.WriteFile(path, []byte("#!/usr/bin/python3\nprint('fixture')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ansiblePythonExecutable(path, []byte("ansible-playbook [core 2.9.27]\n"))
	if err != nil || got != "/usr/bin/python3" {
		t.Fatalf("Ansible shebang Python = %q, %v", got, err)
	}
}

func TestControllerPythonProbeCoversPigstyDependencies(t *testing.T) {
	for _, module := range []string{"yaml", "jinja2", "jmespath", "requests", "cryptography", "OpenSSL"} {
		if !strings.Contains(controllerPythonProbe, module) {
			t.Fatalf("controller Python probe omitted %s", module)
		}
	}
}

func TestVerifyControllerUsesAnsiblePython(t *testing.T) {
	dir := t.TempDir()
	python := filepath.Join(dir, "ansible-python")
	writeBootExecutable(t, python, "#!/bin/sh\nexit 0\n")
	writeBootExecutable(t, filepath.Join(dir, "ansible-playbook"), fmt.Sprintf("#!/bin/sh\necho '  python version = 3.12.0 (fixture) [GCC] (%s)'\n", python))
	for _, name := range []string{"ssh", "sshpass", "sow", "ssh-keygen"} {
		writeBootExecutable(t, filepath.Join(dir, name), "#!/bin/sh\nexit 0\n")
	}
	t.Setenv("PATH", dir)
	if err := verifyController(context.Background()); err != nil {
		t.Fatalf("controller verification failed: %v", err)
	}

	writeBootExecutable(t, python, "#!/bin/sh\necho 'missing jmespath' >&2\nexit 1\n")
	if err := verifyController(context.Background()); err == nil || !strings.Contains(err.Error(), "missing jmespath") {
		t.Fatalf("missing Ansible Python dependency was not reported: %v", err)
	}
}

func writeBootExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
