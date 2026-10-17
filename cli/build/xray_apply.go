package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

type proxyTransaction struct {
	r                        proxyRuntime
	saved                    []proxySnapshot
	active, enabled, mutated bool
}

func (r proxyRuntime) preflightApply(role, listen string, replace bool) error {
	if err := r.checkPlatform(role); err != nil {
		return err
	}
	before, err := readProxyFile(r.config)
	if err != nil {
		return err
	}
	if err := checkProxyPort(r, listen, before); err != nil {
		return err
	}
	if role == "client" {
		b, err := readProxyFile(r.shell)
		if err != nil {
			return err
		}
		if b != nil && !bytes.Equal(b, proxyShell(listen)) && !strings.HasPrefix(string(b), "# Managed by pig build proxy client\n") && !replace {
			return fmt.Errorf("existing proxy shell file differs; replacement requires --replace --yes")
		}
	}
	return nil
}

func (r proxyRuntime) checkServiceUnit() error {
	if r.os != "linux" {
		return nil
	}
	user, err := r.capture("systemctl", "show", "xray", "--property=User", "--value")
	if err != nil || strings.TrimSpace(user) != "xray" {
		return fmt.Errorf("the packaged Xray unit must run as the xray service account")
	}
	group, err := r.capture("systemctl", "show", "xray", "--property=Group", "--value")
	if err != nil || strings.TrimSpace(group) != "" && strings.TrimSpace(group) != "xray" {
		return fmt.Errorf("the packaged Xray unit must use the xray service group")
	}
	command, err := r.capture("systemctl", "show", "xray", "--property=ExecStart", "--value")
	if err != nil || strings.Count(command, "argv[]=") != 1 || !strings.Contains(command, "argv[]=/usr/bin/xray run -config "+r.config+" ;") {
		return fmt.Errorf("existing Xray unit uses a different configuration; refusing to take it over")
	}
	return nil
}

func (r proxyRuntime) stageCandidate(binary string, candidate []byte) error {
	stage, err := os.CreateTemp(filepath.Dir(r.config), ".pig-xray-stage-*.json")
	if err != nil {
		return fmt.Errorf("cannot stage Xray configuration")
	}
	path := stage.Name()
	_ = stage.Close()
	defer os.Remove(path)
	if err := r.writeConfig(path, candidate); err != nil {
		return err
	}
	return r.validateConfig(binary, path)
}

func (r proxyRuntime) transaction(role string) (*proxyTransaction, error) {
	t := &proxyTransaction{r: r, active: r.active(), enabled: r.enabled()}
	paths := []string{r.config, r.serviceFile}
	if role == "client" {
		paths = append(paths, r.shell)
	}
	for _, path := range paths {
		s, err := snapshotProxyFile(path)
		if err != nil {
			return nil, err
		}
		t.saved = append(t.saved, s)
	}
	return t, nil
}

func (t *proxyTransaction) rollback() bool {
	ok := true
	for _, s := range t.saved {
		if s.restore() != nil {
			ok = false
		}
	}
	if t.r.restoreService(t.active, t.enabled) != nil {
		ok = false
	}
	return ok
}

func (r proxyRuntime) restoreService(active, enabled bool) error {
	if r.os == "linux" {
		if err := r.run("systemctl", "daemon-reload"); err != nil {
			return err
		}
		action := "stop"
		if active {
			action = "restart"
		}
		stateErr := r.run("systemctl", action, "xray")
		if !enabled {
			if err := r.run("systemctl", "disable", "xray"); err != nil {
				return err
			}
		}
		return stateErr
	}
	if r.active() {
		if err := r.run("launchctl", "bootout", r.domain()+"/"+r.label); err != nil {
			return err
		}
	}
	var stateErr error
	if active {
		if err := r.run("launchctl", "enable", r.domain()+"/"+r.label); err != nil {
			return err
		}
		stateErr = r.run("launchctl", "bootstrap", r.domain(), r.serviceFile)
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	return errors.Join(stateErr, r.run("launchctl", action, r.domain()+"/"+r.label))
}

func (t *proxyTransaction) commit(candidate, service []byte, changed bool) error {
	if changed {
		t.mutated = true
		if err := t.r.writeConfig(t.r.config, candidate); err != nil {
			return err
		}
		if err := utils.AtomicWriteFile(t.r.serviceFile, service, 0644); err != nil {
			return fmt.Errorf("cannot write Xray service configuration")
		}
	}
	if changed || !t.active || !t.enabled {
		t.mutated = true
		if err := t.r.start(changed && t.active); err != nil {
			return fmt.Errorf("cannot start or enable Xray")
		}
	}
	return nil
}

func (t *proxyTransaction) finishClient(listen string, result *ProxySetupResult) error {
	if err := verifyProxyClient(listen); err != nil {
		return err
	}
	result.HTTP, result.SOCKS, result.PublicVerified = true, true, true
	shell := proxyShell(listen)
	mode := os.FileMode(0600)
	if t.r.os == "linux" {
		mode = 0644
	}
	if !bytes.Equal(t.saved[2].data, shell) || t.saved[2].mode != mode || t.saved[2].uid != os.Geteuid() {
		t.mutated = true
		if err := utils.AtomicWriteFile(t.r.shell, shell, mode); err != nil {
			return fmt.Errorf("cannot write proxy shell helpers")
		}
		result.Changed = true
	}
	logrus.Infof("HTTP and SOCKS proxy ready at %s; source %s then run po", listen, t.r.shell)
	return nil
}

func (r proxyRuntime) configGroupID() (int, error) {
	if r.os != "linux" {
		return os.Getegid(), nil
	}
	group, err := user.LookupGroup("xray")
	if err != nil {
		return 0, fmt.Errorf("packaged xray group is unavailable")
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, fmt.Errorf("packaged xray group is invalid")
	}
	return gid, nil
}

func (r proxyRuntime) configFilesChanged(saved []proxySnapshot, candidate, service []byte, gid int) bool {
	configMode := os.FileMode(0600)
	uid := os.Geteuid()
	groupChanged := false
	if r.os == "linux" {
		configMode = 0640
		uid = 0
		groupChanged = saved[0].gid != gid
	}
	return !bytes.Equal(saved[0].data, candidate) || saved[0].mode != configMode || saved[0].uid != uid || groupChanged || !bytes.Equal(saved[1].data, service) || saved[1].mode != 0644 || saved[1].uid != uid
}

func (r proxyRuntime) binaryPath() (string, error) {
	if r.os != "linux" {
		return exec.LookPath("xray")
	}
	info, err := os.Stat("/usr/bin/xray")
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("packaged xray executable is unavailable")
	}
	return "/usr/bin/xray", nil
}

func (r proxyRuntime) apply(role, listen string, candidate []byte, replace bool, finish func() error) (_ *ProxySetupResult, err error) {
	if err := r.preflightApply(role, listen, replace); err != nil {
		return nil, err
	}
	if err := installXray(); err != nil {
		return nil, err
	}
	binary, err := r.binaryPath()
	if err != nil {
		return nil, fmt.Errorf("xray binary not found after installation")
	}
	if err := r.checkServiceUnit(); err != nil {
		return nil, err
	}
	service, err := r.serviceContent(listen, binary)
	if err != nil {
		return nil, err
	}
	t, err := r.transaction(role)
	if err != nil {
		return nil, err
	}
	if t.saved[1].exists && !r.serviceFileReplaceable(t.saved[1].data, service) && !replace {
		return nil, fmt.Errorf("existing service configuration differs and is not managed by PIG")
	}
	gid, err := r.configGroupID()
	if err != nil {
		return nil, err
	}
	changed := r.configFilesChanged(t.saved, candidate, service, gid)
	if err := os.MkdirAll(filepath.Dir(r.serviceFile), 0755); err != nil {
		return nil, fmt.Errorf("cannot create service configuration directory")
	}
	if err := r.stageCandidate(binary, candidate); err != nil {
		return nil, err
	}
	defer func() {
		if err == nil || !t.mutated {
			return
		}
		if t.rollback() {
			err = fmt.Errorf("%w; original configuration and service state restored", err)
		} else {
			err = fmt.Errorf("%w; rollback incomplete, inspect the Xray service", err)
		}
	}()
	if err := t.commit(candidate, service, changed); err != nil {
		return nil, err
	}
	result := &ProxySetupResult{Role: role, Listen: listen, Config: r.config, Changed: changed || !t.active || !t.enabled}
	if role == "client" {
		if err := t.finishClient(listen, result); err != nil {
			return nil, err
		}
	} else {
		if err := waitProxyListener(r, listen); err != nil {
			return nil, err
		}
		logrus.Info("Xray server ready locally; public ingress requires a client connection test")
	}
	if finish != nil {
		if err := finish(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Match complete generated service files. A marker by itself cannot establish
// ownership: operator edits and foreign files still require --replace --yes.
func (r proxyRuntime) serviceFileReplaceable(data, desired []byte) bool {
	if bytes.Equal(data, desired) {
		return true
	}
	if r.os == "darwin" {
		// c885c696's renderer emitted this exact plist without the later comment.
		legacy := bytes.Replace(desired, []byte("<!-- Managed by pig build proxy -->\n"), nil, 1)
		return bytes.Equal(data, legacy)
	}
	// The only generated Linux variant is the privileged-port capability set.
	for _, listen := range []string{"0.0.0.0:443", "127.0.0.1:9443"} {
		generated, err := r.serviceContent(listen, "/usr/bin/xray")
		if err == nil && bytes.Equal(data, generated) {
			return true
		}
	}
	return false
}
