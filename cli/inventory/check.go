package inventorycli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"pig/internal/output"
	"pig/internal/utils"
	"pig/inventory"
)

const (
	CheckKind = "pig.inventory.check/v1"

	checkProfileLocal   = "local"
	checkProfileAnsible = "ansible"
	checkProfileNetwork = "network"
	checkProfileSSH     = "ssh"

	checkStageValidate  CheckStage = "validate"
	checkStageLocal     CheckStage = "local"
	checkStageAnsible   CheckStage = "ansible"
	checkStageNetwork   CheckStage = "network"
	checkStageSSH       CheckStage = "ssh"
	checkStagePrivilege CheckStage = "privilege"
)

var (
	checkUserPattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*$`)
	ansibleVersionPattern  = regexp.MustCompile(`(?m)^ansible-playbook(?: \[core)?[[:space:]]+v?([0-9][0-9A-Za-z.+_-]*)`)
	openSSHVersionPattern  = regexp.MustCompile(`OpenSSH_([0-9][0-9A-Za-z.p+-]*)`)
	errCheckCommandBlocked = errors.New("check command is not allowlisted")
	checkStageOrder        = map[CheckStage]int{checkStageValidate: 0, checkStageLocal: 1, checkStageAnsible: 2, checkStageNetwork: 3, checkStageSSH: 4, checkStagePrivilege: 5}
	checkProfileOrder      = []string{checkProfileLocal, checkProfileAnsible, checkProfileNetwork, checkProfileSSH}
)

// CheckOptions selects explicit observational profiles. Local validation is
// always enabled. Port and User apply only to network/SSH target probes; Pig
// deliberately does not infer Ansible effective connection variables.
type CheckOptions struct {
	Path     string
	Home     string
	Profiles []string
	Sudo     bool
	Port     int
	User     string
}

// CheckStage is the fixed composite-result stage order from the Inventory contract.
type CheckStage string

// Finding is a redacted, stable Check diagnostic. ResultCode is the MMCCNN
// status selected if this error wins the composite-result ordering.
type Finding struct {
	Code       string             `json:"code" yaml:"code"`
	Severity   inventory.Severity `json:"severity" yaml:"severity"`
	Stage      CheckStage         `json:"stage" yaml:"stage"`
	Path       string             `json:"path,omitempty" yaml:"path,omitempty"`
	Target     string             `json:"target,omitempty" yaml:"target,omitempty"`
	Message    string             `json:"message" yaml:"message"`
	ResultCode int                `json:"result_code,omitempty" yaml:"result_code,omitempty"`
}

// CheckTool records only safe executable identity and a parsed version token.
// Raw command output is never copied into the result.
type CheckTool struct {
	Name      string `json:"name" yaml:"name"`
	Path      string `json:"path,omitempty" yaml:"path,omitempty"`
	Version   string `json:"version,omitempty" yaml:"version,omitempty"`
	Available bool   `json:"available" yaml:"available"`
}

// CheckData is the stable structured Check payload.
type CheckData struct {
	Kind      string      `json:"kind" yaml:"kind"`
	Ready     bool        `json:"ready" yaml:"ready"`
	Inventory string      `json:"inventory" yaml:"inventory"`
	Home      string      `json:"home" yaml:"home"`
	Profiles  []string    `json:"profiles" yaml:"profiles"`
	Targets   []string    `json:"targets" yaml:"targets"`
	Tools     []CheckTool `json:"tools" yaml:"tools"`
	Findings  []Finding   `json:"findings" yaml:"findings"`
}

func (d *CheckData) Text() string {
	if d == nil || len(d.Findings) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(d.Findings))
	for _, finding := range d.Findings {
		location := finding.Path
		if finding.Target != "" {
			location = finding.Target
		}
		rows = append(rows, []string{strings.ToUpper(string(finding.Severity)), string(finding.Stage), finding.Code, location, finding.Message})
	}
	return strings.TrimSuffix(output.RenderTable([]string{"SEVERITY", "STAGE", "CODE", "PATH/TARGET", "MESSAGE"}, rows), "\n")
}

type checkTimeouts struct {
	local     time.Duration
	ansible   time.Duration
	network   time.Duration
	ssh       time.Duration
	privilege time.Duration
}

type checkRuntime struct {
	lookPath        func(string) (string, error)
	stat            func(string) (os.FileInfo, error)
	run             func(context.Context, checkCommand) (checkCommandOutput, error)
	dial            func(context.Context, string, string) (net.Conn, error)
	validateAnsible func(context.Context, string, *inventory.Document) []inventory.Diagnostic
	timeouts        checkTimeouts
	knownHostsFile  string
	identityFile    string
}

type checkCommandKind string

const (
	checkCommandAnsibleVersion checkCommandKind = "ansible-playbook-version"
	checkCommandSSHVersion     checkCommandKind = "ssh-version"
	checkCommandSSH            checkCommandKind = "ssh"
)

type checkCommand struct {
	kind       checkCommandKind
	executable string
	args       []string
	target     string
	user       string
	port       int
	timeout    time.Duration
	privilege  bool
	knownHosts string
	identity   string
}

type checkCommandOutput struct {
	stdout string
	stderr string
}

func defaultCheckRuntime() checkRuntime {
	dialer := &net.Dialer{}
	return checkRuntime{
		lookPath:        exec.LookPath,
		stat:            os.Stat,
		run:             runSystemCheckCommand,
		dial:            dialer.DialContext,
		validateAnsible: inventory.ValidateWithAnsible,
		timeouts: checkTimeouts{
			local: 2 * time.Second, ansible: 10 * time.Second, network: 2 * time.Second,
			ssh: 5 * time.Second, privilege: 5 * time.Second,
		},
	}
}

// Check evaluates the active static Inventory and controller. Network, SSH,
// and privilege observations occur only through explicit profiles/options.
func Check(opts CheckOptions) *output.Result {
	return checkWithRuntime(opts, defaultCheckRuntime())
}

func checkWithRuntime(opts CheckOptions, runtime checkRuntime) *output.Result {
	profiles, enabled, failure := normalizeCheckOptions(&opts)
	if failure != nil {
		return failure
	}

	data := &CheckData{
		Kind: CheckKind, Inventory: strings.TrimSpace(opts.Path), Home: strings.TrimSpace(opts.Home),
		Profiles: profiles, Targets: []string{}, Tools: []CheckTool{}, Findings: []Finding{},
	}

	doc, validationOK, findings := checkInventory(opts.Path)
	data.Findings = append(data.Findings, findings...)
	data.Findings = append(data.Findings, checkLocalLayout(runtime, opts.Home)...)

	ansiblePlaybook, localFindings := checkLocalTool(runtime, "ansible-playbook", checkCommandAnsibleVersion)
	data.Tools = append(data.Tools, ansiblePlaybook)
	data.Findings = append(data.Findings, localFindings...)
	sshTool, localFindings := checkLocalTool(runtime, "ssh", checkCommandSSHVersion)
	data.Tools = append(data.Tools, sshTool)
	data.Findings = append(data.Findings, localFindings...)

	if validationOK {
		data.Targets = declaredCheckTargets(doc)
	}

	if enabled[checkProfileAnsible] {
		ansibleInventory, ansibleFindings := checkAnsibleProfile(runtime, doc, data.Inventory)
		data.Tools = append(data.Tools, ansibleInventory)
		data.Findings = append(data.Findings, ansibleFindings...)
	}
	if validationOK && enabled[checkProfileNetwork] {
		data.Findings = append(data.Findings, checkNetworkTargets(runtime, data.Targets, opts.Port)...)
	}
	if validationOK && enabled[checkProfileSSH] && sshTool.Available {
		data.Findings = append(data.Findings, checkSSHTargets(runtime, sshTool.Path, data.Targets, opts)...)
	}

	data.Findings = deduplicateCheckFindings(data.Findings)
	selected := selectCheckFailure(data.Findings)
	data.Ready = selected == nil
	if selected == nil {
		return output.OK("check passed", data)
	}
	return output.Fail(selected.ResultCode, fmt.Sprintf("check failed at %s: %s", selected.Stage, selected.Code)).WithData(data)
}

func normalizeCheckOptions(opts *CheckOptions) ([]string, map[string]bool, *output.Result) {
	if opts == nil {
		return nil, nil, output.Fail(output.CodeInventoryCheckInvalidArgs, "check options are required")
	}
	if opts.Port == 0 {
		opts.Port = 22
	}
	if opts.Port < 1 || opts.Port > 65535 {
		return nil, nil, output.Fail(output.CodeInventoryCheckInvalidArgs, "check port must be between 1 and 65535")
	}
	opts.User = strings.TrimSpace(opts.User)
	if opts.User != "" && !checkUserPattern.MatchString(opts.User) {
		return nil, nil, output.Fail(output.CodeInventoryCheckInvalidArgs, "check SSH user is invalid")
	}

	enabled := map[string]bool{checkProfileLocal: true}
	for _, raw := range opts.Profiles {
		for _, profile := range strings.Split(raw, ",") {
			profile = strings.ToLower(strings.TrimSpace(profile))
			if profile == "" {
				continue
			}
			switch profile {
			case checkProfileLocal, checkProfileAnsible, checkProfileNetwork, checkProfileSSH:
				enabled[profile] = true
			default:
				return nil, nil, output.Fail(output.CodeInventoryCheckInvalidArgs, fmt.Sprintf("unknown check profile: %s", profile))
			}
		}
	}
	if opts.Sudo && !enabled[checkProfileSSH] {
		return nil, nil, output.Fail(output.CodeInventoryCheckInvalidArgs, "--sudo requires --profile ssh")
	}
	profiles := make([]string, 0, len(enabled))
	for _, profile := range checkProfileOrder {
		if enabled[profile] {
			profiles = append(profiles, profile)
		}
	}
	return profiles, enabled, nil
}

func checkInventory(path string) (*inventory.Document, bool, []Finding) {
	resolved, source, failure := readStaticInventorySource(path, output.CodeInventoryCheckInvalidArgs)
	if failure != nil {
		resultCode := output.CodeInventoryCheckProbeFailed
		code := "CHECK_INVENTORY_SOURCE"
		message := "active Inventory source could not be read"
		switch failure.Code {
		case output.CodeInventoryNotFound:
			resultCode, code, message = output.CodeInventoryCheckResourceMissing, "CHECK_INVENTORY_MISSING", "active Inventory is not configured or does not exist"
		case output.CodeInventoryCheckInvalidArgs:
			resultCode, code, message = output.CodeInventoryCheckValidationFailed, "CHECK_INVENTORY_SOURCE", "active Inventory must be one static YAML file"
		}
		return nil, false, []Finding{newCheckFinding(code, inventory.SeverityError, checkStageValidate, resolved, "", message, resultCode)}
	}
	doc, err := inventory.Parse(resolved, source)
	if err != nil {
		diagnostic := parseDiagnostic(err)
		return nil, false, []Finding{newCheckFinding(diagnostic.Code, diagnostic.Severity, checkStageValidate, diagnostic.Path.String(), "", diagnostic.Message, output.CodeInventoryCheckValidationFailed)}
	}
	diagnostics := doc.ValidateComplete()
	findings := make([]Finding, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		resultCode := 0
		if diagnostic.Severity == inventory.SeverityError {
			resultCode = output.CodeInventoryCheckValidationFailed
		}
		findings = append(findings, newCheckFinding(diagnostic.Code, diagnostic.Severity, checkStageValidate, diagnostic.Path.String(), "", diagnostic.Message, resultCode))
	}
	return doc, !inventory.HasErrors(diagnostics), findings
}

func checkLocalLayout(runtime checkRuntime, home string) []Finding {
	home = strings.TrimSpace(home)
	if home == "" {
		return []Finding{newCheckFinding("CHECK_HOME_MISSING", inventory.SeverityError, checkStageLocal, "", "", "Pigsty home is not configured", output.CodeInventoryCheckResourceMissing)}
	}
	info, err := runtime.stat(home)
	if err != nil {
		return []Finding{checkStatFinding("CHECK_HOME_UNAVAILABLE", home, "Pigsty home is unavailable", err)}
	}
	if !info.IsDir() {
		return []Finding{newCheckFinding("CHECK_HOME_NOT_DIRECTORY", inventory.SeverityError, checkStageLocal, home, "", "Pigsty home is not a directory", output.CodeInventoryCheckResourceMissing)}
	}

	var findings []Finding
	findings = append(findings, checkRequiredPath(runtime, filepath.Join(home, "ansible.cfg"), false, "CHECK_LAYOUT_ANSIBLE_CFG")...)
	findings = append(findings, checkRequiredPath(runtime, filepath.Join(home, "roles"), true, "CHECK_LAYOUT_ROLES")...)
	deployPath := filepath.Join(home, "deploy.yml")
	installPath := filepath.Join(home, "install.yml")
	deploy, deployErr := runtime.stat(deployPath)
	install, installErr := runtime.stat(installPath)
	deployOK := deployErr == nil && deploy.Mode().IsRegular()
	installOK := installErr == nil && install.Mode().IsRegular()
	if !deployOK && !installOK {
		if errors.Is(deployErr, os.ErrPermission) || errors.Is(installErr, os.ErrPermission) {
			findings = append(findings, newCheckFinding("CHECK_LAYOUT_PLAYBOOK", inventory.SeverityError, checkStageLocal, deployPath, "", "Pigsty deployment playbook cannot be inspected", output.CodeInventoryCheckPermissionDenied))
		} else {
			findings = append(findings, newCheckFinding("CHECK_LAYOUT_PLAYBOOK", inventory.SeverityError, checkStageLocal, deployPath, "", "deploy.yml or legacy install.yml is required", output.CodeInventoryCheckResourceMissing))
		}
	}
	return findings
}

func checkRequiredPath(runtime checkRuntime, path string, directory bool, code string) []Finding {
	info, err := runtime.stat(path)
	if err != nil {
		return []Finding{checkStatFinding(code, path, "required Pigsty project path is unavailable", err)}
	}
	valid := info.Mode().IsRegular()
	if directory {
		valid = info.IsDir()
	}
	if !valid {
		return []Finding{newCheckFinding(code, inventory.SeverityError, checkStageLocal, path, "", "required Pigsty project path has the wrong type", output.CodeInventoryCheckResourceMissing)}
	}
	return nil
}

func checkStatFinding(code, path, message string, err error) Finding {
	resultCode := output.CodeInventoryCheckResourceMissing
	if errors.Is(err, os.ErrPermission) {
		resultCode = output.CodeInventoryCheckPermissionDenied
	}
	return newCheckFinding(code, inventory.SeverityError, checkStageLocal, path, "", message, resultCode)
}

func checkLocalTool(runtime checkRuntime, name string, kind checkCommandKind) (CheckTool, []Finding) {
	tool := CheckTool{Name: name}
	executable, err := runtime.lookPath(name)
	if err != nil || strings.TrimSpace(executable) == "" {
		return tool, []Finding{newCheckFinding("CHECK_TOOL_MISSING", inventory.SeverityError, checkStageLocal, name, "", "required local executable is unavailable", output.CodeInventoryCheckDependencyMissing)}
	}
	tool.Available = true
	tool.Path = safeCheckPath(executable)
	args := []string{"--version"}
	if kind == checkCommandSSHVersion {
		args = []string{"-V"}
	}
	command := checkCommand{kind: kind, executable: executable, args: args, timeout: runtime.timeouts.local}
	ctx, cancel := context.WithTimeout(context.Background(), runtime.timeouts.local)
	commandOutput, runErr := runAllowedCheckCommand(ctx, runtime, command)
	deadline := ctx.Err()
	cancel()
	if runErr != nil {
		code, message := "CHECK_TOOL_VERSION_FAILED", "local executable version probe failed"
		if errors.Is(deadline, context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
			code, message = "CHECK_TOOL_VERSION_TIMEOUT", "local executable version probe timed out"
		}
		return tool, []Finding{newCheckFinding(code, inventory.SeverityError, checkStageLocal, name, "", message, output.CodeInventoryCheckProbeFailed)}
	}
	tool.Version = parseCheckVersion(kind, commandOutput.stdout+"\n"+commandOutput.stderr)
	if tool.Version == "" {
		return tool, []Finding{newCheckFinding("CHECK_TOOL_VERSION_UNKNOWN", inventory.SeverityWarning, checkStageLocal, name, "", "local executable version was not recognized", 0)}
	}
	return tool, nil
}

func checkAnsibleProfile(runtime checkRuntime, doc *inventory.Document, path string) (CheckTool, []Finding) {
	tool := CheckTool{Name: "ansible-inventory"}
	executable, err := runtime.lookPath(tool.Name)
	if err != nil || strings.TrimSpace(executable) == "" {
		return tool, []Finding{newCheckFinding("CHECK_ANSIBLE_INVENTORY_MISSING", inventory.SeverityError, checkStageAnsible, tool.Name, "", "ansible-inventory is required by the ansible profile", output.CodeInventoryCheckDependencyMissing)}
	}
	tool.Available = true
	tool.Path = safeCheckPath(executable)
	if doc == nil {
		return tool, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtime.timeouts.ansible)
	diagnostics := runtime.validateAnsible(ctx, executable, doc)
	deadline := ctx.Err()
	cancel()
	if errors.Is(deadline, context.DeadlineExceeded) {
		return tool, []Finding{newCheckFinding("CHECK_ANSIBLE_TIMEOUT", inventory.SeverityError, checkStageAnsible, path, "", "ansible-inventory compatibility probe timed out", output.CodeInventoryCheckProbeFailed)}
	}
	findings := make([]Finding, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		severity := diagnostic.Severity
		resultCode := 0
		if diagnostic.Code == "ANSIBLE_NOT_FOUND" {
			severity = inventory.SeverityError
			resultCode = output.CodeInventoryCheckDependencyMissing
		} else if severity == inventory.SeverityError {
			resultCode = output.CodeInventoryCheckProbeFailed
			if diagnostic.Code == "ANSIBLE_INVENTORY_REJECTED" {
				resultCode = output.CodeInventoryCheckValidationFailed
			}
		}
		diagnosticPath := diagnostic.Path.String()
		if diagnosticPath == "" {
			diagnosticPath = path
		}
		findings = append(findings, newCheckFinding(diagnostic.Code, severity, checkStageAnsible, diagnosticPath, "", diagnostic.Message, resultCode))
	}
	return tool, findings
}

func declaredCheckTargets(doc *inventory.Document) []string {
	if doc == nil {
		return []string{}
	}
	semantic := doc.Inventory()
	if semantic == nil || semantic.All == nil {
		return []string{}
	}
	seen := make(map[string]struct{})
	var visit func(*inventory.Group)
	visit = func(group *inventory.Group) {
		if group == nil {
			return
		}
		for _, host := range group.Hosts {
			if host != nil {
				seen[host.Name] = struct{}{}
			}
		}
		for _, child := range group.Children {
			visit(child)
		}
	}
	visit(semantic.All)
	targets := make([]string, 0, len(seen))
	for target := range seen {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func checkNetworkTargets(runtime checkRuntime, targets []string, port int) []Finding {
	var findings []Finding
	for _, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), runtime.timeouts.network)
		connection, err := runtime.dial(ctx, "tcp", net.JoinHostPort(target, strconv.Itoa(port)))
		deadline := ctx.Err()
		if connection != nil {
			_ = connection.Close()
		}
		cancel()
		if err == nil {
			continue
		}
		code, message := "CHECK_NETWORK_UNREACHABLE", "TCP reachability probe failed"
		if errors.Is(deadline, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			code, message = "CHECK_NETWORK_TIMEOUT", "TCP reachability probe timed out"
		}
		findings = append(findings, newCheckFinding(code, inventory.SeverityError, checkStageNetwork, "", target, message, output.CodeInventoryCheckNetworkFailed))
	}
	return findings
}

func checkSSHTargets(runtime checkRuntime, executable string, targets []string, opts CheckOptions) []Finding {
	var findings []Finding
	for _, target := range targets {
		sshOK, finding := runSSHCheckProbe(runtime, executable, target, opts.User, opts.Port, false)
		if finding != nil {
			findings = append(findings, *finding)
		}
		if !sshOK || !opts.Sudo {
			continue
		}
		_, finding = runSSHCheckProbe(runtime, executable, target, opts.User, opts.Port, true)
		if finding != nil {
			findings = append(findings, *finding)
		}
	}
	return findings
}

func runSSHCheckProbe(runtime checkRuntime, executable, target, user string, port int, privilege bool) (bool, *Finding) {
	timeout := runtime.timeouts.ssh
	stage := checkStageSSH
	resultCode := output.CodeInventoryCheckSSHFailed
	if privilege {
		timeout = runtime.timeouts.privilege
		stage = checkStagePrivilege
		resultCode = output.CodeInventoryCheckPermissionDenied
	}
	args, err := buildSSHCheckArgs(target, user, port, timeout, privilege, runtime.knownHostsFile, runtime.identityFile)
	if err != nil {
		finding := newCheckFinding("CHECK_SSH_TARGET_INVALID", inventory.SeverityError, stage, "", target, "SSH target could not be represented safely", output.CodeInventoryCheckInvalidArgs)
		return false, &finding
	}
	command := checkCommand{
		kind: checkCommandSSH, executable: executable, args: args,
		target: target, user: user, port: port, timeout: timeout, privilege: privilege,
		knownHosts: runtime.knownHostsFile, identity: runtime.identityFile,
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	_, runErr := runAllowedCheckCommand(ctx, runtime, command)
	deadline := ctx.Err()
	cancel()
	if runErr == nil {
		return true, nil
	}
	code, message := "CHECK_SSH_FAILED", "allowlisted SSH observation failed"
	if privilege {
		code, message = "CHECK_SUDO_DENIED", "non-interactive sudo observation failed"
	}
	if errors.Is(deadline, context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
		code = "CHECK_SSH_TIMEOUT"
		message = "allowlisted SSH observation timed out"
		if privilege {
			code, message = "CHECK_SUDO_TIMEOUT", "non-interactive sudo observation timed out"
		}
	}
	if errors.Is(runErr, errCheckCommandBlocked) {
		code, message, resultCode = "CHECK_COMMAND_BLOCKED", "probe command did not match the Check allowlist", output.CodeInventoryCheckProbeFailed
	}
	finding := newCheckFinding(code, inventory.SeverityError, stage, "", target, message, resultCode)
	return false, &finding
}

// buildSSHCheckArgs is the documented SSH allowlist. -F /dev/null prevents
// user configuration from adding proxy/local commands; strict host checking
// and UpdateHostKeys=no prevent known_hosts writes. The only remote commands
// are exactly `true` and, with separate consent, `sudo -n true`.
func buildSSHCheckArgs(target, user string, port int, timeout time.Duration, privilege bool, knownHosts, identity string) ([]string, error) {
	if ip := net.ParseIP(target); ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("target must be an IPv4 address")
	}
	if user != "" && !checkUserPattern.MatchString(user) {
		return nil, fmt.Errorf("invalid SSH user")
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SSH port")
	}
	seconds := int((timeout + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	destination := target
	if user != "" {
		destination = user + "@" + target
	}
	args := []string{
		"-F", "/dev/null", "-T", "-n",
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "NumberOfPasswordPrompts=0",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UpdateHostKeys=no",
		"-o", "ConnectionAttempts=1",
		"-o", "ConnectTimeout=" + strconv.Itoa(seconds),
		"-o", "ClearAllForwardings=yes",
		"-o", "ForwardAgent=no",
		"-o", "PermitLocalCommand=no",
		"-o", "ProxyCommand=none",
		"-o", "ProxyJump=none",
		"-o", "ControlMaster=no",
		"-o", "ControlPath=none",
		"-o", "ControlPersist=no",
		"-o", "AddKeysToAgent=no",
		"-o", "LogLevel=ERROR",
	}
	if knownHosts != "" {
		if !safeCheckProbePath(knownHosts) {
			return nil, fmt.Errorf("invalid known_hosts path")
		}
		args = append(args, "-o", "UserKnownHostsFile="+knownHosts)
	}
	if identity != "" {
		if !safeCheckProbePath(identity) {
			return nil, fmt.Errorf("invalid identity path")
		}
		args = append(args, "-o", "IdentitiesOnly=yes", "-i", identity)
	}
	args = append(args, "-p", strconv.Itoa(port), "--", destination)
	if privilege {
		return append(args, "sudo", "-n", "true"), nil
	}
	return append(args, "true"), nil
}

func runAllowedCheckCommand(ctx context.Context, runtime checkRuntime, command checkCommand) (checkCommandOutput, error) {
	if !isAllowedCheckCommand(command) {
		return checkCommandOutput{}, errCheckCommandBlocked
	}
	return runtime.run(ctx, command)
}

func isAllowedCheckCommand(command checkCommand) bool {
	if strings.TrimSpace(command.executable) == "" {
		return false
	}
	switch command.kind {
	case checkCommandAnsibleVersion:
		return slices.Equal(command.args, []string{"--version"})
	case checkCommandSSHVersion:
		return slices.Equal(command.args, []string{"-V"})
	case checkCommandSSH:
		expected, err := buildSSHCheckArgs(command.target, command.user, command.port, command.timeout, command.privilege, command.knownHosts, command.identity)
		return err == nil && slices.Equal(command.args, expected)
	default:
		return false
	}
}

func runSystemCheckCommand(ctx context.Context, command checkCommand) (checkCommandOutput, error) {
	cmd := exec.CommandContext(ctx, command.executable, command.args...)
	if command.kind == checkCommandSSH {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		return checkCommandOutput{}, cmd.Run()
	}
	stdout, stderr := utils.NewLimitedBuffer(4096), utils.NewLimitedBuffer(4096)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	return checkCommandOutput{stdout: stdout.String(), stderr: stderr.String()}, err
}

func parseCheckVersion(kind checkCommandKind, value string) string {
	var match []string
	switch kind {
	case checkCommandAnsibleVersion:
		match = ansibleVersionPattern.FindStringSubmatch(value)
	case checkCommandSSHVersion:
		match = openSSHVersionPattern.FindStringSubmatch(value)
	}
	if len(match) < 2 {
		return ""
	}
	if kind == checkCommandSSHVersion {
		return "OpenSSH_" + match[1]
	}
	return match[1]
}

func safeCheckPath(path string) string {
	for _, char := range path {
		if char < 0x20 || char == 0x7f {
			return ""
		}
	}
	return filepath.Clean(path)
}

func safeCheckProbePath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for _, char := range path {
		if char <= 0x20 || char == 0x7f {
			return false
		}
	}
	return filepath.Clean(path) == path
}

func newCheckFinding(code string, severity inventory.Severity, stage CheckStage, path, target, message string, resultCode int) Finding {
	return Finding{Code: code, Severity: severity, Stage: stage, Path: path, Target: target, Message: message, ResultCode: resultCode}
}

func deduplicateCheckFindings(findings []Finding) []Finding {
	sort.SliceStable(findings, func(i, j int) bool {
		left, right := findings[i], findings[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Target != right.Target {
			return left.Target < right.Target
		}
		if checkSeverityRank(left.Severity) != checkSeverityRank(right.Severity) {
			return checkSeverityRank(left.Severity) > checkSeverityRank(right.Severity)
		}
		if checkStageRank(left.Stage) != checkStageRank(right.Stage) {
			return checkStageRank(left.Stage) < checkStageRank(right.Stage)
		}
		if left.ResultCode != right.ResultCode {
			return left.ResultCode < right.ResultCode
		}
		return left.Message < right.Message
	})
	seen := make(map[string]struct{}, len(findings))
	result := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		key := finding.Code + "\x00" + finding.Path + "\x00" + finding.Target
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, finding)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if checkStageRank(left.Stage) != checkStageRank(right.Stage) {
			return checkStageRank(left.Stage) < checkStageRank(right.Stage)
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Target < right.Target
	})
	return result
}

func selectCheckFailure(findings []Finding) *Finding {
	var selected *Finding
	for index := range findings {
		candidate := &findings[index]
		if candidate.Severity != inventory.SeverityError || candidate.ResultCode == 0 {
			continue
		}
		if selected == nil || checkFindingPrecedes(*candidate, *selected) {
			selected = candidate
		}
	}
	return selected
}

func checkFindingPrecedes(left, right Finding) bool {
	if checkSeverityRank(left.Severity) != checkSeverityRank(right.Severity) {
		return checkSeverityRank(left.Severity) > checkSeverityRank(right.Severity)
	}
	if checkStageRank(left.Stage) != checkStageRank(right.Stage) {
		return checkStageRank(left.Stage) < checkStageRank(right.Stage)
	}
	if left.Code != right.Code {
		return left.Code < right.Code
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	return left.Target < right.Target
}

func checkSeverityRank(severity inventory.Severity) int {
	switch severity {
	case inventory.SeverityError:
		return 3
	case inventory.SeverityWarning:
		return 2
	default:
		return 1
	}
}

func checkStageRank(stage CheckStage) int {
	if rank, ok := checkStageOrder[stage]; ok {
		return rank
	}
	return len(checkStageOrder)
}
