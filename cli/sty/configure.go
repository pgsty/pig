package sty

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"pig/cli/ext"
	"pig/cli/get"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"pig/inventory"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConfigureOutput = "pigsty.yml"
	defaultTemplateMode    = "meta"
	defaultPrimaryIP       = "10.10.10.10"
	defaultNoProxy         = "localhost,127.0.0.1,10.0.0.0/8,192.168.0.0/16,*.pigsty,*.aliyun.com,mirrors.*,mirror.*,*.tsinghua.edu.cn"
	defaultCheckTimeout    = 3 * time.Second
)

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

const ConfigureKind = "pig.sty.configure/v1"

// ConfigureData is structured output for native configure execution.
type ConfigureData struct {
	Kind               string   `json:"kind" yaml:"kind"`
	Mode               string   `json:"mode" yaml:"mode"`
	TemplatePath       string   `json:"template_path" yaml:"template_path"`
	OutputPath         string   `json:"output_path" yaml:"output_path"`
	Region             string   `json:"region" yaml:"region"`
	PrimaryIP          string   `json:"primary_ip" yaml:"primary_ip"`
	SSHPort            string   `json:"ssh_port,omitempty" yaml:"ssh_port,omitempty"`
	PGVersion          string   `json:"pg_version,omitempty" yaml:"pg_version,omitempty"` // requested version, retained for compatibility
	EffectivePGVersion string   `json:"effective_pg_version,omitempty" yaml:"effective_pg_version,omitempty"`
	Native             bool     `json:"native" yaml:"native"`
	GeneratedSecrets   []string `json:"generated_secrets,omitempty" yaml:"generated_secrets,omitempty"`
	Warnings           []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

const (
	configureANSIReset  = "\033[0m"
	configureANSIRed    = "\033[0;31m"
	configureANSIGreen  = "\033[0;32m"
	configureANSIYellow = "\033[0;33m"
	configureANSIBlue   = "\033[0;34m"
)

type configureTextStyle struct {
	color bool
}

func (s configureTextStyle) paint(colorCode, text string) string {
	if !s.color {
		return text
	}
	return colorCode + text + configureANSIReset
}

func (s configureTextStyle) ok(msg string) string {
	if !s.color {
		return "[ OK ] " + msg
	}
	return "[" + s.paint(configureANSIGreen, " OK ") + "] " + s.paint(configureANSIGreen, msg)
}

func (s configureTextStyle) warn(msg string) string {
	if !s.color {
		return "[WARN] " + msg
	}
	return "[" + s.paint(configureANSIYellow, "WARN") + "] " + s.paint(configureANSIYellow, msg)
}

func (s configureTextStyle) fail(msg string) string {
	if !s.color {
		return "[FAIL] " + msg
	}
	return "[" + s.paint(configureANSIRed, "FAIL") + "] " + s.paint(configureANSIRed, msg)
}

func (s configureTextStyle) hint(msg string) string {
	return s.paint(configureANSIBlue, msg)
}

func formatConfigureKV(key, value string) string {
	return fmt.Sprintf("%-10s = %s", key, value)
}

// Text renders ConfigureData in text output mode.
func (d *ConfigureData) Text() string {
	if d == nil {
		return ""
	}

	style := configureTextStyle{color: output.IsColorEnabled()}
	lines := make([]string, 0, 16+len(d.Warnings))

	lines = append(lines, style.hint(fmt.Sprintf("configure pigsty %s begin", config.PigstyVersion)))
	lines = append(lines, style.ok(formatConfigureKV("mode", d.Mode)))
	lines = append(lines, style.ok(formatConfigureKV("template", d.TemplatePath)))
	lines = append(lines, style.ok(formatConfigureKV("output", d.OutputPath)))
	lines = append(lines, style.ok(formatConfigureKV("region", d.Region)))
	lines = append(lines, style.ok(formatConfigureKV("primary_ip", d.PrimaryIP)))
	if d.SSHPort != "" {
		lines = append(lines, style.ok(formatConfigureKV("ssh_port", d.SSHPort)))
	}
	if d.PGVersion != "" {
		lines = append(lines, style.ok(formatConfigureKV("pg_version", d.PGVersion)))
	}
	if d.EffectivePGVersion != "" && d.EffectivePGVersion != d.PGVersion {
		lines = append(lines, style.ok(formatConfigureKV("effective_pg_version", d.EffectivePGVersion)))
	}
	if len(d.GeneratedSecrets) > 0 {
		lines = append(lines, style.ok(formatConfigureKV("passwords", fmt.Sprintf("%d generated", len(d.GeneratedSecrets)))))
	}
	if len(d.Warnings) > 0 {
		for _, w := range d.Warnings {
			if warning := strings.TrimSpace(w); warning != "" {
				lines = append(lines, style.warn(warning))
			}
		}
	}
	lines = append(lines, style.warn("don't forget to check it and change passwords!"))
	if d.OutputPath == "" {
		lines = append(lines, style.fail("output path is empty"))
	} else if filepath.Base(d.OutputPath) == defaultConfigureOutput {
		lines = append(lines, style.hint("proceed with ./deploy.yml"))
	} else {
		lines = append(lines, style.hint(fmt.Sprintf("proceed with ./deploy.yml -i %s", d.OutputPath)))
	}
	return strings.Join(lines, "\n")
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

	// Beta majors get a heads-up: the generic templates are not tuned for them.
	if ext.IsBetaPGMajor(pgVer) {
		betaMode := fmt.Sprintf("pg%d", pgVer)
		if mode != betaMode {
			warn := fmt.Sprintf("PG%d is a beta major, template conf/%s.yml is not tuned for it", pgVer, mode)
			if _, statErr := os.Stat(filepath.Join(home, "conf", betaMode+".yml")); statErr == nil {
				warn = fmt.Sprintf("PG%d is a beta major, consider the dedicated template: pig sty conf -c %s", pgVer, betaMode)
			}
			warnings = append(warnings, warn)
		}
	}

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

	mutationResult, err := mutateTemplateDocument(string(templateBytes), mutationOptions{
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
	warnings = append(warnings, mutationResult.Warnings...)
	candidate := mutationResult.Document
	diagnostics := candidate.Validate()
	if inventory.HasErrors(diagnostics) {
		return output.Fail(output.CodeStyConfigureFailed, formatInventoryDiagnostics("rendered inventory is invalid", diagnostics))
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == inventory.SeverityWarning {
			warnings = append(warnings, fmt.Sprintf("%s at %s: %s", diagnostic.Code, diagnostic.Path.String(), diagnostic.Message))
		}
	}
	if ansibleInventory, lookupErr := exec.LookPath("ansible-inventory"); lookupErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		externalDiagnostics := inventory.ValidateWithAnsible(ctx, ansibleInventory, candidate)
		cancel()
		if inventory.HasErrors(externalDiagnostics) {
			return output.Fail(output.CodeStyConfigureFailed, formatInventoryDiagnostics("ansible-inventory rejected rendered inventory", externalDiagnostics))
		}
	} else if !opts.DisablePreflight {
		warnings = append(warnings, "ansible-inventory not found, external inventory validation skipped")
	}

	if err := utils.PutFileMode(outputPath, candidate.Bytes(), 0600); err != nil {
		return output.Fail(output.CodeStyConfigureWriteFailed, fmt.Sprintf("failed to write output: %v", err))
	}

	data := &ConfigureData{
		Kind:               ConfigureKind,
		Mode:               mode,
		TemplatePath:       templatePath,
		OutputPath:         outputPath,
		Region:             region,
		PrimaryIP:          primaryIP,
		SSHPort:            strings.TrimSpace(opts.SSHPort),
		PGVersion:          strings.TrimSpace(opts.PGVersion),
		EffectivePGVersion: formatOptionalPGVersion(mutationResult.EffectivePGVersion),
		Native:             true,
		GeneratedSecrets:   mutationResult.Generated,
		Warnings:           warnings,
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
	if !ext.IsInstallablePGMajor(v) {
		return 0, fmt.Errorf("invalid pg major version: %s (valid: %s)", ver, ext.PostgresInstallableVersionString())
	}
	return v, nil
}

func resolvePrimaryIP(opts ConfigureOptions, mode string) (string, []string, error) {
	var warnings []string
	if opts.Skip && strings.TrimSpace(opts.PrimaryIP) != "" {
		return "", warnings, errors.New("--skip and --ip are mutually exclusive")
	}
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
			warnings = append(warnings, "mode=meta on Ubuntu 20.04 (EOL 2025-04-23), consider Ubuntu 22.04/24.04/26.04")
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
		if commandExists("dnf") || (config.OSMajor == 7 && commandExists("yum")) || commandExists("zypper") || commandExists("brew") {
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
		if commandExists("dnf") || (config.OSMajor == 7 && commandExists("yum")) || commandExists("zypper") || commandExists("brew") {
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
	return strings.TrimSpace(currentUserFn())
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

type templateMutationResult struct {
	Document           *inventory.Document
	EffectivePGVersion int
	Generated          []string
	Warnings           []string
}

func mutateTemplateDocument(content string, opts mutationOptions) (*templateMutationResult, error) {
	doc, err := inventory.Parse(opts.Mode+".yml", []byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse inventory template: %w", err)
	}
	mutation := doc.Mutate()
	globals := mutation.AllVars()
	var warnings []string

	if !strings.HasPrefix(opts.Mode, "build/") && opts.PrimaryIP != "" && opts.PrimaryIP != defaultPrimaryIP {
		if _, err := mutation.ReplaceAll(defaultPrimaryIP, opts.PrimaryIP,
			inventory.WithLiteralTargets(inventory.LiteralAll),
			inventory.WithIdentifier("primary_ip")); err != nil {
			return nil, err
		}
	}

	if opts.CPUCount > 0 && opts.CPUCount < 4 {
		if scalarEquals(globals, "node_tune", "oltp") {
			if err := globals.Set("node_tune", "tiny"); err != nil {
				return nil, err
			}
		}
		if scalarEquals(globals, "pg_conf", "oltp.yml") {
			if err := globals.Set("pg_conf", "tiny.yml"); err != nil {
				return nil, err
			}
		}
	}

	if opts.Region != "" && opts.Region != "default" {
		insertOptions := insertAfterExisting(globals, "admin_ip")
		if err := globals.Set("region", opts.Region, insertOptions...); err != nil {
			return nil, err
		}
	}
	if opts.Region == "china" {
		for _, key := range []string{"docker_registry_mirrors", "PIP_MIRROR_URL"} {
			if err := enableChinaTemplateKey(mutation, key); err != nil {
				return nil, err
			}
		}
	}

	if len(opts.Proxy) > 0 {
		if err := upsertProxyEnv(mutation, opts.Proxy); err != nil {
			return nil, err
		}
	}

	pinnedKernelMode := opts.Mode == "mssql" || opts.Mode == "polar"
	pgMajorTemplate := pgMajorTemplateMode.MatchString(opts.Mode)
	templatePGVersion, _ := mappingInteger(globals, "pg_version")
	if templatePGVersion == 0 && pgMajorTemplate {
		templatePGVersion, _ = strconv.Atoi(strings.TrimPrefix(opts.Mode, "pg"))
	}
	effectivePGVersion := templatePGVersion
	if opts.PGVersion > 0 && !pinnedKernelMode && !pgMajorTemplate {
		insertOptions := insertAfterExisting(globals, "node_repo_modules")
		if err := globals.Set("pg_version", opts.PGVersion, insertOptions...); err != nil {
			return nil, err
		}
		effectivePGVersion = opts.PGVersion
		if templatePGVersion > 0 && templatePGVersion != opts.PGVersion {
			if _, err := mutation.ReplaceAll(fmt.Sprintf("pg%d-", templatePGVersion), fmt.Sprintf("pg%d-", opts.PGVersion),
				inventory.WithLiteralTargets(inventory.LiteralScalars|inventory.LiteralComments),
				inventory.WithIdentifier("pg_package_alias")); err != nil {
				return nil, err
			}
		}
		warnings = append(warnings, pgVersionOverrideWarnings(mutation.Document(), opts.PGVersion)...)
	} else if opts.PGVersion > 0 && opts.PGVersion != effectivePGVersion {
		if effectivePGVersion > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"pg_version %d ignored: %s template pins PostgreSQL %d", opts.PGVersion, opts.Mode, effectivePGVersion))
		} else {
			warnings = append(warnings, fmt.Sprintf(
				"pg_version %d ignored: %s template pins its own PostgreSQL version", opts.PGVersion, opts.Mode))
		}
	}

	// Beta majors beyond the stable window ship from the PGDG testing repos.
	// Only all.vars module lists are changed; group/host overrides remain intact.
	if ext.IsBetaPGMajor(effectivePGVersion) {
		enabled, err := enableBetaRepoModules(mutation)
		if err != nil {
			return nil, err
		}
		if !enabled {
			warnings = append(warnings, fmt.Sprintf(
				"beta repo module not enabled: no all.vars repo module list with 'pgsql' in template, add 'beta' manually for PG%d packages", effectivePGVersion))
		}
	}

	if shouldSetLocale(effectivePGVersion, opts.Mode, opts.LocaleAvailable) {
		if err := ensureLocaleSettings(mutation); err != nil {
			return nil, err
		}
	}

	var generated []string
	if opts.GeneratePassword {
		generated, err = replacePasswordsInMutation(mutation)
		if err != nil {
			return nil, err
		}
	}

	committed, _, err := mutation.Commit()
	if err != nil {
		if opts.GeneratePassword {
			return nil, fmt.Errorf("failed to commit inventory edits after secret generation")
		}
		return nil, fmt.Errorf("failed to commit inventory edits: %w", err)
	}
	return &templateMutationResult{
		Document: committed, EffectivePGVersion: effectivePGVersion, Generated: generated, Warnings: warnings,
	}, nil
}

func enableChinaTemplateKey(mutation *inventory.Mutation, key string) error {
	doc := mutation.Document()
	before := doc.CommentedKeyCount(key)
	activeBefore := doc.PathsByKey(key)
	if before == 0 && len(activeBefore) == 0 {
		// Most templates do not declare PIP_MIRROR_URL, and specialized
		// inventories may not manage Docker at all. Match the legacy configure
		// contract: activate every declaration that exists, but do not invent a
		// template-specific value when the key is absent.
		return nil
	}
	result, err := mutation.UncommentKey(key)
	if err != nil {
		return err
	}
	if result.Matched != before || result.Changed != before {
		return fmt.Errorf("china mirror key %s was not activated completely: matched=%d changed=%d expected=%d", key, result.Matched, result.Changed, before)
	}
	if remaining := mutation.Document().CommentedKeyCount(key); remaining != 0 {
		return fmt.Errorf("china mirror key %s still has %d inactive declaration(s)", key, remaining)
	}
	if len(result.Paths) != result.Changed {
		return fmt.Errorf("china mirror key %s produced %d paths for %d edits", key, len(result.Paths), result.Changed)
	}
	activeAfter := mutation.Document().PathsByKey(key)
	if len(activeAfter) == 0 {
		return fmt.Errorf("china mirror key %s has no active declaration", key)
	}
	for _, path := range activeAfter {
		if !variableDeclarationPath(path) {
			return fmt.Errorf("china mirror key %s activated outside vars at %s", key, path.String())
		}
	}
	return nil
}

func variableDeclarationPath(path inventory.Path) bool {
	limit := len(path) - 1
	for index := 0; index < limit; index++ {
		segment := path[index]
		if segment.Kind != inventory.KeySegment {
			continue
		}
		if segment.Key == "vars" || (segment.Key == "hosts" && index+2 < len(path)) {
			return true
		}
	}
	return false
}

func pgVersionOverrideWarnings(doc *inventory.Document, target int) []string {
	if doc == nil || target <= 0 {
		return nil
	}
	global := inventory.AllVars("pg_version").String()
	var warnings []string
	for _, path := range doc.PathsByKey("pg_version") {
		if path.String() == global {
			continue
		}
		value, ok := doc.Lookup(path)
		if !ok || value == nil || value.Scalar == nil || fmt.Sprint(value.Scalar.Value) == strconv.Itoa(target) {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("pg_version override at %s remains %v while all.vars requests %d", path.String(), value.Scalar.Value, target))
	}
	return warnings
}

var pgMajorTemplateMode = regexp.MustCompile(`^pg[0-9]+$`)

func mappingInteger(mapping *inventory.MappingEditor, key string) (int, bool) {
	value, ok := mapping.Lookup(key)
	if !ok || value == nil || value.Scalar == nil || value.Scalar.Kind != inventory.IntegerScalar {
		return 0, false
	}
	switch number := value.Scalar.Value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), int64(int(number)) == number
	case uint:
		return int(number), uint(int(number)) == number
	case uint64:
		return int(number), uint64(int(number)) == number
	default:
		return 0, false
	}
}

func formatOptionalPGVersion(version int) string {
	if version <= 0 {
		return ""
	}
	return strconv.Itoa(version)
}

func scalarEquals(mapping *inventory.MappingEditor, key, expected string) bool {
	value, ok := mapping.Lookup(key)
	return ok && value != nil && value.Scalar != nil && fmt.Sprint(value.Scalar.Value) == expected
}

func insertAfterExisting(mapping *inventory.MappingEditor, key string) []inventory.InsertOption {
	if _, ok := mapping.Lookup(key); ok {
		return []inventory.InsertOption{inventory.AfterKey(key)}
	}
	return nil
}

func upsertProxyEnv(mutation *inventory.Mutation, proxy map[string]string) error {
	globals := mutation.AllVars()
	keys := []string{"http_proxy", "https_proxy", "all_proxy", "no_proxy"}
	value, exists := globals.Lookup("proxy_env")
	insertOptions := insertAfterExisting(globals, "region")
	if len(insertOptions) == 0 {
		insertOptions = insertAfterExisting(globals, "admin_ip")
	}
	proxyVars := globals.Mapping("proxy_env", insertOptions...)
	if !exists {
		if err := proxyVars.Ensure(); err != nil {
			return err
		}
		value, _ = globals.Lookup("proxy_env")
	}
	if value == nil || (value.Kind != inventory.MappingValue && value.Kind != inventory.NullValue) {
		return fmt.Errorf("all.vars.proxy_env must be a mapping")
	}
	for _, key := range keys {
		desired := strings.TrimSpace(proxy[key])
		if desired != "" {
			if err := proxyVars.Set(key, desired); err != nil {
				return err
			}
			continue
		}
		if _, exists := proxyVars.Lookup(key); exists {
			if err := proxyVars.Delete(key); err != nil {
				return err
			}
		}
	}
	return nil
}

func enableBetaRepoModules(mutation *inventory.Mutation) (bool, error) {
	globals := mutation.AllVars()
	enabled := false
	for _, key := range []string{"node_repo_modules", "repo_modules"} {
		value, ok := globals.Lookup(key)
		if !ok || value == nil || value.Scalar == nil {
			continue
		}
		modules := strings.Split(fmt.Sprint(value.Scalar.Value), ",")
		for i := range modules {
			modules[i] = strings.TrimSpace(modules[i])
		}
		if slices.Contains(modules, "beta") {
			enabled = true
			continue
		}
		pgsql := slices.Index(modules, "pgsql")
		if pgsql < 0 {
			continue
		}
		if err := globals.Set(key, strings.Join(slices.Insert(modules, pgsql+1, "beta"), ",")); err != nil {
			return false, err
		}
		enabled = true
	}
	return enabled, nil
}

func shouldSetLocale(pgVersion int, mode string, localeAvailable bool) bool {
	return (pgVersion == 0 && mode != "mssql" && mode != "polar") || pgVersion >= 17 || localeAvailable
}

func ensureLocaleSettings(mutation *inventory.Mutation) error {
	globals := mutation.AllVars()
	if _, exists := globals.Lookup("pg_version"); !exists {
		return nil
	}
	locales := []struct {
		key     string
		comment string
	}{
		{key: "pg_locale", comment: "overwrite default C locale"},
		{key: "pg_lc_collate", comment: "overwrite default C lc_collate"},
		{key: "pg_lc_ctype", comment: "overwrite default C lc_ctype"},
	}
	anchor := "pg_version"
	for _, locale := range locales {
		if err := globals.Set(locale.key, "C.UTF-8", inventory.AfterKey(anchor), inventory.WithInlineComment(locale.comment)); err != nil {
			return err
		}
		anchor = locale.key
	}
	return nil
}

func replacePasswordsInMutation(mutation *inventory.Mutation) ([]string, error) {
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
		paths := mutation.Document().PathsByKey(key)
		if len(paths) == 0 {
			continue
		}
		pass, err := randomPassword(24)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			if err := mutation.Set(path, pass); err != nil {
				return nil, err
			}
		}
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
			return nil, err
		}
		count, err := mutation.ReplaceAll(token, pass,
			inventory.WithLiteralTargets(inventory.LiteralScalars),
			inventory.WithIdentifier(token))
		if err != nil {
			return nil, err
		}
		if count > 0 {
			generated = append(generated, token)
		}
	}
	return generated, nil
}

func formatInventoryDiagnostics(prefix string, diagnostics []inventory.Diagnostic) string {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == inventory.SeverityError {
			return fmt.Sprintf("%s: %s at %s: %s", prefix, diagnostic.Code, diagnostic.Path.String(), diagnostic.Message)
		}
	}
	return prefix
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
