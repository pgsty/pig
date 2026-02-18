package sty

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigureOutput = "pigsty.yml"
	defaultTemplateMode    = "meta"
	defaultPrimaryIP       = "10.10.10.10"
	defaultNoProxy         = "localhost,127.0.0.1,10.0.0.0/8,192.168.0.0/16,*.pigsty,*.aliyun.com,mirrors.*,mirror.*,*.tsinghua.edu.cn"
	defaultCheckTimeout    = 3 * time.Second
)

var validPGMajorVersions = []int{13, 14, 15, 16, 17, 18}

var (
	lookPathFn               = exec.LookPath
	runCommandFn             = runCommandWithTimeout
	currentUserFn            = detectCurrentUser
	shellOutputFn            = utils.ShellOutput
	regionDetectFn           = detectRegionFromNetworkCondition
	inReader       io.Reader = os.Stdin
	errWriter      io.Writer = os.Stderr
	goosFn                   = detectedGOOS
	goarchFn                 = detectedGOARCH
	localIPv4sFn             = localIPv4s

	networkConditionFn = get.NetworkCondition
	internetAccessFn   = func() bool { return get.InternetAccess }
	networkSourceFn    = func() string { return get.Source }
	networkRegionFn    = func() string { return get.Region }
)

// ConfigureOptions contains native configure options.
type ConfigureOptions struct {
	PigstyHome string
	Mode       string
	PrimaryIP  string
	PGVersion  string
	Region     string
	SSHPort    string
	OutputFile string

	Skip           bool
	UseProxy       bool
	NonInteractive bool
	Generate       bool

	// Optional test hooks. 0 means runtime detection.
	CPUCount         int
	LocaleAvailable  *bool
	DisablePreflight bool
}

// ConfigureData is structured output for native configure execution.
type ConfigureData struct {
	Mode             string   `json:"mode" yaml:"mode"`
	TemplatePath     string   `json:"template_path" yaml:"template_path"`
	OutputPath       string   `json:"output_path" yaml:"output_path"`
	Region           string   `json:"region" yaml:"region"`
	PrimaryIP        string   `json:"primary_ip" yaml:"primary_ip"`
	SSHPort          string   `json:"ssh_port,omitempty" yaml:"ssh_port,omitempty"`
	PGVersion        string   `json:"pg_version,omitempty" yaml:"pg_version,omitempty"`
	Native           bool     `json:"native" yaml:"native"`
	GeneratedSecrets []string `json:"generated_secrets,omitempty" yaml:"generated_secrets,omitempty"`
	Warnings         []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// Text renders ConfigureData in text output mode.
func (d *ConfigureData) Text() string {
	if d == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("mode: %s\n", d.Mode))
	sb.WriteString(fmt.Sprintf("template: %s\n", d.TemplatePath))
	sb.WriteString(fmt.Sprintf("output: %s\n", d.OutputPath))
	sb.WriteString(fmt.Sprintf("region: %s\n", d.Region))
	sb.WriteString(fmt.Sprintf("primary_ip: %s\n", d.PrimaryIP))
	if d.SSHPort != "" {
		sb.WriteString(fmt.Sprintf("ssh_port: %s\n", d.SSHPort))
	}
	if d.PGVersion != "" {
		sb.WriteString(fmt.Sprintf("pg_version: %s\n", d.PGVersion))
	}
	if len(d.GeneratedSecrets) > 0 {
		sb.WriteString(fmt.Sprintf("generated_secrets: %d\n", len(d.GeneratedSecrets)))
	}
	if len(d.Warnings) > 0 {
		sb.WriteString("warnings:\n")
		for _, w := range d.Warnings {
			sb.WriteString("  - ")
			sb.WriteString(w)
			sb.WriteByte('\n')
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ConfigureNative generates pigsty config with native Go implementation.
func ConfigureNative(opts ConfigureOptions) *output.Result {
	home := strings.TrimSpace(opts.PigstyHome)
	if home == "" {
		return output.Fail(output.CodeStyConfigureInvalidArgs, "pigsty home is required")
	}

	mode, err := normalizeConfigureMode(opts.Mode)
	if err != nil {
		return output.Fail(output.CodeStyConfigureInvalidArgs, err.Error())
	}

	templatePath, err := resolveTemplatePath(home, mode)
	if err != nil {
		return output.Fail(output.CodeStyConfigureInvalidArgs, err.Error())
	}
	if _, err := os.Stat(templatePath); err != nil {
		return output.Fail(output.CodeStyConfigureTemplateNotFound, fmt.Sprintf("template not found: conf/%s.yml", mode))
	}

	pgVer, err := validatePGVersion(opts.PGVersion)
	if err != nil {
		return output.Fail(output.CodeStyConfigureInvalidArgs, err.Error())
	}

	region := strings.TrimSpace(strings.ToLower(opts.Region))
	if region == "" {
		region = detectRegion()
	}

	primaryIP, warnings, err := resolvePrimaryIP(opts, mode)
	if err != nil {
		return output.Fail(output.CodeStyConfigureInvalidArgs, err.Error())
	}
	preflightWarnings, err := runPreflightChecks(opts, mode, primaryIP)
	if err != nil {
		return output.Fail(output.CodeStyConfigureFailed, err.Error())
	}
	warnings = append(warnings, preflightWarnings...)

	outputPath := resolveOutputPath(home, opts.OutputFile)
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return output.Fail(output.CodeStyConfigureFailed, fmt.Sprintf("failed to read template: %v", err))
	}

	cpuCount := opts.CPUCount
	if cpuCount <= 0 {
		if config.NodeCPUCount > 0 {
			cpuCount = config.NodeCPUCount
		} else {
			cpuCount = runtime.NumCPU()
		}
	}
	localeAvailable := detectLocaleAvailable(opts.LocaleAvailable)
	proxy := buildProxyEnv(opts.UseProxy)

	mutated, generated, err := mutateTemplate(string(templateBytes), mutationOptions{
		Mode:             mode,
		PrimaryIP:        primaryIP,
		Region:           region,
		PGVersion:        pgVer,
		Proxy:            proxy,
		CPUCount:         cpuCount,
		LocaleAvailable:  localeAvailable,
		GeneratePassword: opts.Generate,
	})
	if err != nil {
		return output.Fail(output.CodeStyConfigureFailed, err.Error())
	}

	var yamlCheck interface{}
	if err := yaml.Unmarshal([]byte(mutated), &yamlCheck); err != nil {
		return output.Fail(output.CodeStyConfigureFailed, fmt.Sprintf("rendered yaml is invalid: %v", err))
	}

	if err := utils.PutFile(outputPath, []byte(mutated)); err != nil {
		return output.Fail(output.CodeStyConfigureWriteFailed, fmt.Sprintf("failed to write output: %v", err))
	}

	data := &ConfigureData{
		Mode:             mode,
		TemplatePath:     templatePath,
		OutputPath:       outputPath,
		Region:           region,
		PrimaryIP:        primaryIP,
		SSHPort:          strings.TrimSpace(opts.SSHPort),
		PGVersion:        strings.TrimSpace(opts.PGVersion),
		Native:           true,
		GeneratedSecrets: generated,
		Warnings:         warnings,
	}

	msg := "pigsty configured"
	if outputPath != filepath.Join(home, defaultConfigureOutput) {
		msg = fmt.Sprintf("pigsty configured @ %s", outputPath)
	}
	return output.OK(msg, data)
}

func normalizeConfigureMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ReplaceAll(mode, "\\", "/"))
	if mode == "" {
		return defaultTemplateMode, nil
	}
	clean := filepath.ToSlash(filepath.Clean(mode))
	if clean == "." || clean == "" {
		return defaultTemplateMode, nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("invalid conf mode: %s", mode)
	}
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '/' {
			continue
		}
		return "", fmt.Errorf("invalid conf mode: %s", mode)
	}
	return clean, nil
}

func resolveTemplatePath(home, mode string) (string, error) {
	confDir := filepath.Join(home, "conf")
	target := filepath.Join(confDir, filepath.FromSlash(mode)+".yml")

	baseAbs, err := filepath.Abs(confDir)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid conf mode path: %s", mode)
	}
	return targetAbs, nil
}

func validatePGVersion(ver string) (int, error) {
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(ver)
	if err != nil {
		return 0, fmt.Errorf("invalid pg major version: %s", ver)
	}
	if !slices.Contains(validPGMajorVersions, v) {
		return 0, fmt.Errorf("invalid pg major version: %s (valid: 13,14,15,16,17,18)", ver)
	}
	return v, nil
}

func resolvePrimaryIP(opts ConfigureOptions, mode string) (string, []string, error) {
	var warnings []string
	if strings.HasPrefix(mode, "build/") {
		warnings = append(warnings, "primary_ip replacement skipped for build/* templates")
		return defaultPrimaryIP, warnings, nil
	}
	if opts.Skip {
		return defaultPrimaryIP, warnings, nil
	}

	ip := strings.TrimSpace(opts.PrimaryIP)
	if ip != "" {
		if parsed := net.ParseIP(ip); parsed == nil || parsed.To4() == nil {
			return "", warnings, fmt.Errorf("invalid primary ip: %s", ip)
		}
		return ip, warnings, nil
	}

	if goosFn() == "darwin" {
		warnings = append(warnings, "macOS detected, fallback to placeholder primary_ip")
		return defaultPrimaryIP, warnings, nil
	}

	ips := localIPv4sFn()
	switch len(ips) {
	case 0:
		warnings = append(warnings, "failed to probe local IPv4, fallback to placeholder primary_ip")
		return defaultPrimaryIP, warnings, nil
	case 1:
		return ips[0], warnings, nil
	default:
		for _, c := range ips {
			if c == defaultPrimaryIP {
				return defaultPrimaryIP, warnings, nil
			}
		}
	}

	if opts.NonInteractive {
		return "", warnings, fmt.Errorf("multiple IP candidates found, specify --ip or disable --non-interactive")
	}

	fmt.Fprintf(errWriter, "INPUT primary_ip address (e.g. %s):\n=> ", defaultPrimaryIP)
	var input string
	if _, err := fmt.Fscanln(inReader, &input); err != nil {
		return "", warnings, fmt.Errorf("failed to read primary ip: %w", err)
	}
	input = strings.TrimSpace(input)
	if parsed := net.ParseIP(input); parsed == nil || parsed.To4() == nil {
		return "", warnings, fmt.Errorf("invalid primary ip: %s", input)
	}
	return input, warnings, nil
}

func localIPv4s() []string {
	out := make([]string, 0, 4)
	seen := make(map[string]struct{})
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			s := ip.String()
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

func resolveOutputPath(home, outputFile string) string {
	outputFile = strings.TrimSpace(outputFile)
	if outputFile == "" {
		return filepath.Join(home, defaultConfigureOutput)
	}
	if filepath.IsAbs(outputFile) {
		return outputFile
	}
	return filepath.Join(home, outputFile)
}

func detectRegion() string {
	return regionDetectFn()
}

// detectRegionFromNetworkCondition reuses existing get.NetworkCondition()
// probing logic for region inference.
func detectRegionFromNetworkCondition() string {
	_ = networkConditionFn()
	region := strings.TrimSpace(strings.ToLower(networkRegionFn()))
	if region != "" {
		return region
	}
	if !internetAccessFn() {
		return "default"
	}
	if networkSourceFn() == get.ViaCC {
		return "china"
	}
	return "default"
}

func detectLocaleAvailable(override *bool) bool {
	if override != nil {
		return *override
	}
	out, err := shellOutputFn("locale", "-a")
	if err != nil {
		return false
	}
	lower := strings.ToLower(out)
	return strings.Contains(lower, "c.utf8") || strings.Contains(lower, "c.utf-8")
}

func buildProxyEnv(useProxy bool) map[string]string {
	if !useProxy {
		return nil
	}
	httpProxy := strings.TrimSpace(os.Getenv("HTTP_PROXY"))
	if httpProxy == "" {
		httpProxy = strings.TrimSpace(os.Getenv("http_proxy"))
	}
	httpsProxy := strings.TrimSpace(os.Getenv("HTTPS_PROXY"))
	allProxy := strings.TrimSpace(os.Getenv("ALL_PROXY"))
	noProxy := strings.TrimSpace(os.Getenv("NO_PROXY"))
	if noProxy == "" {
		noProxy = defaultNoProxy
	}
	if allProxy != "" && httpProxy == "" {
		httpProxy = allProxy
	}
	if allProxy != "" && httpsProxy == "" {
		httpsProxy = allProxy
	}

	proxy := map[string]string{}
	if httpProxy != "" {
		proxy["http_proxy"] = httpProxy
	}
	if httpsProxy != "" {
		proxy["https_proxy"] = httpsProxy
	}
	if allProxy != "" {
		proxy["all_proxy"] = allProxy
	}
	if noProxy != "" {
		proxy["no_proxy"] = noProxy
	}
	return proxy
}

func runPreflightChecks(opts ConfigureOptions, mode, primaryIP string) ([]string, error) {
	if opts.DisablePreflight {
		return nil, nil
	}
	warnings := make([]string, 0, 8)
	warnings = append(warnings, checkKernelWarnings()...)
	warnings = append(warnings, checkMachineWarnings()...)
	warnings = append(warnings, checkAutoModeWarnings(opts.Mode)...)

	packageWarnings, err := checkPackageManagerWarnings()
	if err != nil {
		return warnings, err
	}
	warnings = append(warnings, packageWarnings...)
	warnings = append(warnings, checkVendorVersionWarnings()...)
	warnings = append(warnings, checkSudoWarnings(opts.Skip)...)
	warnings = append(warnings, checkLocalSSHWarnings(opts.SSHPort)...)
	warnings = append(warnings, checkAdminWarnings(mode, primaryIP, opts.Skip, opts.SSHPort)...)
	warnings = append(warnings, checkAnsibleWarnings()...)
	return warnings, nil
}

func checkKernelWarnings() []string {
	switch goosFn() {
	case "linux":
		return nil
	case "darwin":
		return []string{"kernel=darwin, this node can be used as admin node only"}
	default:
		return []string{fmt.Sprintf("kernel=%s is not officially supported, Linux is recommended", goosFn())}
	}
}

func checkMachineWarnings() []string {
	switch goarchFn() {
	case "amd64", "arm64":
		return nil
	default:
		return []string{fmt.Sprintf("machine architecture=%s is not officially supported", goarchFn())}
	}
}

func checkAutoModeWarnings(requestedMode string) []string {
	if strings.TrimSpace(requestedMode) != "" || goosFn() != "linux" {
		return nil
	}
	vendor := strings.ToLower(strings.TrimSpace(config.OSVendor))
	version := strings.TrimSpace(config.OSVersion)
	warnings := make([]string, 0, 2)

	switch vendor {
	case "centos":
		if version == "7" {
			warnings = append(warnings, "mode=meta on CentOS 7.9 (EOL 2024-06-30), consider EL9/EL10")
		}
	case "debian":
		if version == "11" {
			warnings = append(warnings, "mode=meta on Debian 11 (EOL 2024-08-14), consider Debian 12/13")
		}
	case "ubuntu":
		if version == "20" {
			warnings = append(warnings, "mode=meta on Ubuntu 20.04 (EOL 2025-04-23), consider Ubuntu 22.04/24.04")
		}
	}
	if vendor == "" {
		warnings = append(warnings, "OS vendor unknown, fallback to conf/meta.yml")
	}
	return warnings
}

func checkPackageManagerWarnings() ([]string, error) {
	if goosFn() != "linux" {
		return nil, nil
	}
	switch strings.TrimSpace(config.OSType) {
	case config.DistroDEB:
		if commandExists("apt") || commandExists("apt-get") || commandExists("brew") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to determine os package manager for deb system")
	case config.DistroEL:
		if commandExists("dnf") || commandExists("yum") || commandExists("zypper") || commandExists("brew") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to determine os package manager for rpm system")
	}

	if commandExists("dpkg") {
		if commandExists("apt") || commandExists("apt-get") || commandExists("brew") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to determine os package manager for deb system")
	}
	if commandExists("rpm") {
		if commandExists("dnf") || commandExists("yum") || commandExists("zypper") || commandExists("brew") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to determine os package manager for rpm system")
	}
	return []string{"failed to determine os package type (dpkg/rpm not found)"}, nil
}

func checkVendorVersionWarnings() []string {
	if goosFn() != "linux" {
		return nil
	}
	if strings.TrimSpace(config.OSVendor) == "" || strings.TrimSpace(config.OSVersionFull) == "" {
		return []string{"os release is unknown, distro-specific defaults may be inaccurate"}
	}
	return nil
}

func checkSudoWarnings(skip bool) []string {
	if goosFn() == "darwin" {
		return nil
	}
	currentUser := effectiveCurrentUser()
	if currentUser == "" {
		currentUser = "current-user"
	}
	if currentUser == "root" {
		return nil
	}
	if !commandExists("sudo") {
		return []string{"sudo command not found, nopasswd sudo check skipped"}
	}
	if err := runCommandFn(2*time.Second, "sudo", "-n", "ls"); err != nil {
		if skip {
			return []string{fmt.Sprintf("sudo=%s may require password", currentUser)}
		}
		return []string{fmt.Sprintf("sudo=%s missing nopasswd, configure sudoers for deployment", currentUser)}
	}
	return nil
}

func checkLocalSSHWarnings(port string) []string {
	if goosFn() == "darwin" {
		return nil
	}
	if !commandExists("ssh") {
		return []string{"ssh command not found, localhost ssh check skipped"}
	}
	currentUser := effectiveCurrentUser()
	target := "127.0.0.1"
	if currentUser != "" {
		target = fmt.Sprintf("%s@127.0.0.1", currentUser)
	}
	args := []string{"-oBatchMode=yes", "-o", "StrictHostKeyChecking no", "-o", "ConnectTimeout=2"}
	if port = strings.TrimSpace(port); port != "" {
		args = append(args, "-p", port)
	}
	args = append(args, target, "ls")
	if err := runCommandFn(3*time.Second, "ssh", args...); err != nil {
		if port != "" {
			return []string{fmt.Sprintf("ssh localhost check failed on port %s", port)}
		}
		return []string{"ssh localhost check failed"}
	}
	return nil
}

func checkAdminWarnings(mode, primaryIP string, skip bool, port string) []string {
	if goosFn() == "darwin" {
		return nil
	}
	if strings.HasPrefix(mode, "build/") {
		return []string{"admin ssh/sudo check skipped for build/* templates"}
	}
	if skip {
		return []string{"admin ssh/sudo check skipped due to --skip"}
	}
	warnings := make([]string, 0, 2)
	if !commandExists("ssh") {
		warnings = append(warnings, "ssh command not found, admin ssh/sudo check skipped")
		return warnings
	}
	args := []string{"-oBatchMode=yes", "-o", "StrictHostKeyChecking no", "-o", "ConnectTimeout=2"}
	if port = strings.TrimSpace(port); port != "" {
		args = append(args, "-p", port)
	}
	args = append(args, primaryIP, "sudo", "-n", "ls")
	if err := runCommandFn(4*time.Second, "ssh", args...); err != nil {
		if port != "" {
			warnings = append(warnings, fmt.Sprintf("admin ssh/sudo check failed for %s:%s", primaryIP, port))
		} else {
			warnings = append(warnings, fmt.Sprintf("admin ssh/sudo check failed for %s", primaryIP))
		}
	}
	if effectiveCurrentUser() == "root" {
		warnings = append(warnings, "user=root is not recommended")
	}
	return warnings
}

func checkAnsibleWarnings() []string {
	if commandExists("ansible-playbook") {
		return nil
	}
	return []string{"ansible-playbook not found, consider running `pig sty boot` first"}
}

func commandExists(name string) bool {
	_, err := lookPathFn(name)
	return err == nil
}

func effectiveCurrentUser() string {
	u := strings.TrimSpace(currentUserFn())
	if u != "" {
		return u
	}
	if u = strings.TrimSpace(config.CurrentUser); u != "" {
		return u
	}
	if cu, err := user.Current(); err == nil && cu != nil {
		if name := strings.TrimSpace(cu.Username); name != "" {
			return name
		}
	}
	if env := strings.TrimSpace(os.Getenv("USER")); env != "" {
		return env
	}
	return ""
}

func detectCurrentUser() string {
	if u := strings.TrimSpace(config.CurrentUser); u != "" {
		return u
	}
	if cu, err := user.Current(); err == nil && cu != nil {
		return strings.TrimSpace(cu.Username)
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

func runCommandWithTimeout(timeout time.Duration, name string, args ...string) error {
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s timeout", name)
		}
		return err
	}
	return nil
}

func detectedGOOS() string {
	if v := strings.TrimSpace(strings.ToLower(config.GOOS)); v != "" {
		return v
	}
	return runtime.GOOS
}

func detectedGOARCH() string {
	if v := strings.TrimSpace(strings.ToLower(config.OSArch)); v != "" {
		return v
	}
	if v := strings.TrimSpace(strings.ToLower(config.GOARCH)); v != "" {
		return v
	}
	return runtime.GOARCH
}

type mutationOptions struct {
	Mode             string
	PrimaryIP        string
	Region           string
	PGVersion        int
	Proxy            map[string]string
	CPUCount         int
	LocaleAvailable  bool
	GeneratePassword bool
}

func mutateTemplate(content string, opts mutationOptions) (string, []string, error) {
	if !strings.HasPrefix(opts.Mode, "build/") {
		content = strings.ReplaceAll(content, defaultPrimaryIP, opts.PrimaryIP)
	}

	if opts.CPUCount > 0 && opts.CPUCount < 4 {
		content = strings.ReplaceAll(content, "pg_conf: oltp.yml", "pg_conf: tiny.yml")
		content = strings.ReplaceAll(content, "node_tune: oltp", "node_tune: tiny")
	}

	if opts.Region != "" && opts.Region != "default" {
		content = strings.ReplaceAll(content, "    region: default", "    region: "+opts.Region)
	}
	if opts.Region == "china" {
		content = strings.ReplaceAll(content, "#docker_registry_mirrors", "docker_registry_mirrors")
		content = strings.ReplaceAll(content, "#PIP_MIRROR_URL", "PIP_MIRROR_URL")
	}

	if len(opts.Proxy) > 0 {
		content = removeProxyBlock(content)
		content = insertProxyBlock(content, opts.Proxy)
	}

	if opts.PGVersion > 0 && opts.Mode != "mssql" && opts.Mode != "polar" {
		content = upsertPGVersion(content, opts.PGVersion)
		content = strings.ReplaceAll(content, "pg18-", fmt.Sprintf("pg%d-", opts.PGVersion))
	}

	if shouldSetLocale(opts.PGVersion, opts.Mode, opts.LocaleAvailable) {
		content = insertLocaleSettings(content)
	}

	// Keep this branch for forward-compat parity with legacy configure logic.
	// It becomes active once validPGMajorVersions includes 19+.
	if opts.PGVersion >= 19 {
		content = strings.ReplaceAll(content, "node,infra,pgsql", "node,infra,pgsql,beta")
	}

	generated := []string{}
	if opts.GeneratePassword {
		var err error
		content, generated, err = replacePasswords(content)
		if err != nil {
			return "", nil, err
		}
	}

	return content, generated, nil
}

func removeProxyBlock(content string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^[ \t]*proxy_env:.*\n?`),
		regexp.MustCompile(`(?m)^[ \t]*https?_proxy:.*\n?`),
		regexp.MustCompile(`(?m)^[ \t]*all_proxy:.*\n?`),
		regexp.MustCompile(`(?m)^[ \t]*no_proxy:.*\n?`),
	}
	for _, re := range patterns {
		content = re.ReplaceAllString(content, "")
	}
	return content
}

func insertProxyBlock(content string, proxy map[string]string) string {
	order := []string{"http_proxy", "https_proxy", "all_proxy", "no_proxy"}
	lines := []string{"    proxy_env:"}
	for _, k := range order {
		if v, ok := proxy[k]; ok && v != "" {
			lines = append(lines, fmt.Sprintf("      %s: %q", k, v))
		}
	}
	block := strings.Join(lines, "\n")

	regionLine := regexp.MustCompile(`(?m)^    region:.*$`)
	loc := regionLine.FindStringIndex(content)
	if loc == nil {
		if strings.HasSuffix(content, "\n") {
			return content + block + "\n"
		}
		return content + "\n" + block + "\n"
	}
	return content[:loc[1]] + "\n" + block + content[loc[1]:]
}

func upsertPGVersion(content string, version int) string {
	re := regexp.MustCompile(`(?m)^[ \t]*pg_version:.*$`)
	line := fmt.Sprintf("    pg_version: %d                      # configured pg major version", version)
	if re.MatchString(content) {
		return re.ReplaceAllString(content, line)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + line + "\n"
}

func shouldSetLocale(pgVersion int, mode string, localeAvailable bool) bool {
	if (pgVersion == 0 && mode != "mssql" && mode != "polar") || pgVersion >= 17 {
		return true
	}
	return localeAvailable
}

func insertLocaleSettings(content string) string {
	if strings.Contains(content, "pg_locale: C.UTF-8") {
		return content
	}
	localeBlock := "    pg_locale: C.UTF-8                  # overwrite default C local\n" +
		"    pg_lc_collate: C.UTF-8              # overwrite default C lc_collate\n" +
		"    pg_lc_ctype: C.UTF-8                # overwrite default C lc_ctype\n"

	re := regexp.MustCompile(`(?m)^    pg_version:.*$`)
	loc := re.FindStringIndex(content)
	if loc == nil {
		// Keep parity with legacy sed behavior: locale block is inserted only
		// when a pg_version line exists.
		return content
	}
	return content[:loc[1]] + "\n" + localeBlock + content[loc[1]:]
}

func replacePasswords(content string) (string, []string, error) {
	prefixKeys := []string{
		"grafana_admin_password",
		"pg_admin_password",
		"pg_monitor_password",
		"pg_replication_password",
		"patroni_password",
		"haproxy_admin_password",
		"minio_secret_key",
		"etcd_root_password",
	}
	generated := make([]string, 0, len(prefixKeys)+7)
	for _, key := range prefixKeys {
		pass, err := randomPassword(24)
		if err != nil {
			return "", nil, err
		}
		re := regexp.MustCompile(`(?m)^(\s*` + regexp.QuoteMeta(key) + `:\s*).*$`)
		content = re.ReplaceAllString(content, "${1}"+pass)
		generated = append(generated, key)
	}

	globalTokens := []string{
		"DBUser.Meta",
		"DBUser.Viewer",
		"S3User.Backup",
		"S3User.Meta",
		"S3User.Data",
		"DBUser.Supa",
		"Vibe.Coding",
	}
	for _, token := range globalTokens {
		pass, err := randomPassword(24)
		if err != nil {
			return "", nil, err
		}
		content = strings.ReplaceAll(content, token, pass)
		generated = append(generated, token)
	}
	return content, generated, nil
}

var passwordAlphabet = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

func randomPassword(length int) (string, error) {
	if length <= 0 {
		length = 24
	}
	out := make([]byte, length)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}
