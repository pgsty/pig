package sty

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const controllerPythonProbe = `
import importlib

missing = []
for name in ('yaml', 'jinja2', 'jmespath', 'requests'):
    try:
        importlib.import_module(name)
    except ImportError:
        missing.append(name)

crypto = False
for name in ('cryptography', 'OpenSSL'):
    try:
        importlib.import_module(name)
        crypto = True
        break
    except ImportError:
        pass
if not crypto:
    missing.append('cryptography|OpenSSL')

if missing:
    raise ImportError('missing controller Python modules: ' + ', '.join(missing))
`

// verifyController checks the executable surface as well as Python modules
// imported by Pigsty playbooks. The Python probe follows ansible-playbook's
// interpreter rather than assuming that /usr/bin/python3 is the same runtime.
func verifyController(ctx context.Context) error {
	ansiblePath, err := exec.LookPath("ansible-playbook")
	if err != nil {
		return fmt.Errorf("ansible-playbook: %w", err)
	}
	version, err := runControllerProbe(ctx, ansiblePath, "--version")
	if err != nil {
		return err
	}
	python, err := ansiblePythonExecutable(ansiblePath, version)
	if err != nil {
		return err
	}
	if _, err := runControllerProbe(ctx, python, "-c", controllerPythonProbe); err != nil {
		return fmt.Errorf("Ansible Python dependencies: %w", err)
	}

	for _, probe := range [][]string{
		{"ssh", "-V"},
		{"sshpass", "-V"},
		{"sow", "--version"},
	} {
		path, err := exec.LookPath(probe[0])
		if err != nil {
			return fmt.Errorf("%s: %w", probe[0], err)
		}
		if _, err := runControllerProbe(ctx, path, probe[1:]...); err != nil {
			return err
		}
	}
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		return fmt.Errorf("ssh-keygen: %w", err)
	}
	return nil
}

func runControllerProbe(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return output, fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return output, fmt.Errorf("%s: %w: %s", filepath.Base(name), err, detail)
}

func ansiblePythonExecutable(ansiblePath string, versionOutput []byte) (string, error) {
	for _, line := range strings.Split(string(versionOutput), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "python version =") || !strings.HasSuffix(line, ")") {
			continue
		}
		if start := strings.LastIndex(line, " ("); start >= 0 {
			candidate := strings.TrimSpace(line[start+2 : len(line)-1])
			if filepath.IsAbs(candidate) && strings.Contains(strings.ToLower(filepath.Base(candidate)), "python") {
				return candidate, nil
			}
		}
	}

	resolved, err := filepath.EvalSymlinks(ansiblePath)
	if err != nil {
		resolved = ansiblePath
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect ansible-playbook interpreter: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return "", fmt.Errorf("inspect ansible-playbook interpreter: missing shebang")
	}
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "#!"))
	if len(fields) == 0 {
		return "", fmt.Errorf("inspect ansible-playbook interpreter: invalid shebang")
	}
	interpreter := fields[0]
	if filepath.Base(interpreter) == "env" {
		if len(fields) < 2 {
			return "", fmt.Errorf("inspect ansible-playbook interpreter: invalid env shebang")
		}
		interpreter, err = exec.LookPath(fields[1])
		if err != nil {
			return "", fmt.Errorf("find ansible-playbook interpreter %s: %w", fields[1], err)
		}
	}
	if !strings.Contains(strings.ToLower(filepath.Base(interpreter)), "python") {
		return "", fmt.Errorf("ansible-playbook does not use a Python interpreter: %s", interpreter)
	}
	return interpreter, nil
}
