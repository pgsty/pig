package sty

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"pig/cli/ext"
	"pig/cli/install"
	"pig/cli/repo"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
)

const BootResultKind = "pig.sty.boot/v2"

type NativeBootstrapOptions struct {
	Region      string
	PackagePath string
	Keep        bool
}

type BootData struct {
	Kind               string   `json:"kind" yaml:"kind"`
	Ansible            string   `json:"ansible" yaml:"ansible"`
	Mode               string   `json:"mode" yaml:"mode"`
	PackageManager     string   `json:"package_manager" yaml:"package_manager"`
	RepositoryPolicy   string   `json:"repository_policy" yaml:"repository_policy"`
	RepositoryRollback string   `json:"repository_rollback,omitempty" yaml:"repository_rollback,omitempty"`
	PackagePath        string   `json:"package_path,omitempty" yaml:"package_path,omitempty"`
	RepositoryDir      string   `json:"repository_dir,omitempty" yaml:"repository_dir,omitempty"`
	LocalSSH           string   `json:"local_ssh,omitempty" yaml:"local_ssh,omitempty"`
	Locale             string   `json:"locale,omitempty" yaml:"locale,omitempty"`
	PigstyHome         string   `json:"pigsty_home,omitempty" yaml:"pigsty_home,omitempty"`
	PigstyInit         string   `json:"pigsty_init,omitempty" yaml:"pigsty_init,omitempty"`
	Changed            bool     `json:"changed" yaml:"changed"`
	Warnings           []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	next               []output.NextAction
}

func (d *BootData) Text() string {
	if d == nil {
		return ""
	}
	style := configureTextStyle{color: output.IsColorEnabled()}
	lines := make([]string, 0, len(d.Warnings)+len(d.next)+1)
	for _, warning := range d.Warnings {
		lines = append(lines, style.warn(warning))
	}
	if len(d.next) > 0 {
		lines = append(lines, style.hint("proceed with:"))
		commandWidth := 0
		for _, action := range d.next {
			commandWidth = max(commandWidth, len(action.Command))
		}
		commandWidth += 2
		for _, action := range d.next {
			hint := fmt.Sprintf("%-*s # %s", commandWidth, action.Command, action.Reason)
			lines = append(lines, "    "+style.hint(hint))
		}
	}
	return strings.Join(lines, "\n")
}

var (
	bootInstall                   = install.InstallPackagesWithOptions
	bootRepos                     = repo.ConfigureRepos
	bootResetRepos                = repo.RmRepos
	bootRepo                      = repo.ExtractOfflineWithResult
	bootAddLocal                  = repo.AddLocalRepo
	bootRestoreRepos              = repo.RestoreRepos
	bootLocalRepo                 = repo.LocalRepoPath
	bootLookPath                  = exec.LookPath
	bootEUID                      = os.Geteuid
	bootDownload                  = utils.DownloadFile
	bootRepairLocale              = repairDebianBootstrapLocale
	bootVerifyAnsible             = verifyAnsible
	bootVerifyHelpers             = verifyControllerHelpers
	bootStdinIsTerminal           = utils.StdinIsTerminal
	bootEnsureSSH                 = ensureLocalSSH
	bootEnsurePigsty              = ensureDefaultPigsty
	bootAutoPkg                   = "/tmp/pkg.tgz"
	bootRepoDir                   = "/www/pigsty"
	bootProgressOut     io.Writer = os.Stderr
)

var errUntrustedAutomaticPackage = errors.New("untrusted automatic offline package")

// BootstrapNative is deliberately a small coordinator. Repository rendering,
// package alias resolution, sudo command execution, and package installation
// remain owned by their existing packages.
func BootstrapNative(ctx context.Context, opts NativeBootstrapOptions) *output.Result {
	// Match the legacy bootstrap contract: existing repositories are backed up
	// and replaced unless the caller explicitly asks to keep them.
	overwrite := !opts.Keep
	policy := "overwrite"
	if !overwrite {
		policy = "keep"
	}
	// Region flows into repository routing, mirror selection, and the child
	// pig sty init invocation; normalize once so "China" and "china" behave
	// identically on every path.
	opts.Region = strings.ToLower(strings.TrimSpace(opts.Region))
	data := &BootData{Kind: BootResultKind, RepositoryPolicy: policy}
	if bootEUID() != 0 {
		return output.Fail(output.CodeStyBootstrapPermissionDenied, "native bootstrap requires root privileges").WithData(data)
	}
	prepareBootstrapLocale(ctx, data)

	ansibleErr := bootVerifyAnsible(ctx)
	if ansibleErr != nil {
		data.Ansible = "missing"
	} else {
		data.Ansible = "ready"
	}
	pkg, hasLocalRepo, sourceFailure := resolveBootstrapOfflineSource(opts.PackagePath, data)
	if sourceFailure != nil {
		return sourceFailure
	}

	if ansibleErr != nil {
		if failure := bootstrapAnsible(opts, data, pkg, hasLocalRepo); failure != nil {
			return failure
		}
		// The locales package may only have arrived with the controller
		// package set, so a repair that failed before installation gets one
		// retry before Ansible is verified again.
		retryBootstrapLocale(ctx, data)
		if err := bootVerifyAnsible(ctx); err != nil {
			data.Ansible = "failed"
			return output.Fail(output.CodeStyBootstrapFailed, "ansible bootstrap verification failed").
				WithDetail(conciseBootMessage(err)).WithData(data)
		}
		data.Ansible = "bootstrapped"
		data.Changed = true
		writeBootStatus("ansible", "ready")
	} else {
		if pkg != "" || hasLocalRepo {
			if failure := prepareOfflineBootstrap(opts, data, pkg, hasLocalRepo, false); failure != nil {
				return failure
			}
		} else {
			data.Mode = "ready"
		}
		writeBootStatus("ansible", "ready")
	}

	// Advisory only: deploy needs these helpers, but an otherwise usable
	// controller is not failed solely because one of them is absent.
	if err := bootVerifyHelpers(ctx); err != nil {
		appendBootWarning(data, "controller helpers", err)
	}

	sshStatus, sshErr := bootEnsureSSH(ctx)
	data.LocalSSH = sshStatus
	if sshErr != nil {
		data.LocalSSH = "warning"
		appendBootWarning(data, "localhost SSH", sshErr)
	} else {
		if sshStatus == "repaired" {
			data.Changed = true
		}
		writeBootStatus("ssh", "ok")
	}

	pigsty, pigstyErr := bootEnsurePigsty(ctx, opts.Region)
	data.PigstyHome = pigsty.Path
	data.PigstyInit = pigsty.Status
	if pigsty.Warning != "" {
		appendBootWarningText(data, pigsty.Warning)
	}
	if pigstyErr != nil {
		if data.PigstyInit == "" {
			data.PigstyInit = pigstyInitFailed
		}
		appendBootWarning(data, "Pigsty init", pigstyErr)
	} else if pigsty.Status == pigstyInitInitialized || pigsty.Status == pigstyInitLocal {
		data.Changed = true
	}
	if pigstyErr == nil && pigsty.Warning == "" {
		writeBootStatus("pigsty", displayPigstyStatus(pigsty.Status))
	}

	data.next = []output.NextAction{
		{Command: "pig sty conf -g", Reason: "generate config with random passwords"},
		{Command: "pig inv edit", Reason: "edit the inventory, [OPTIONAL]"},
		{Command: "pig sty deploy", Reason: "run the actual deployment"},
	}
	writeBootOK("bootstrap complete")
	result := output.OK("Pigsty bootstrap complete", data)
	result.WithNextActions(data.next...)
	return result
}

func resolveBootstrapOfflineSource(requested string, data *BootData) (string, bool, *output.Result) {
	requested = strings.TrimSpace(requested)
	hasLocalRepo := regularFile(filepath.Join(bootRepoDir, "repo_complete"))
	if hasLocalRepo && requested != "" {
		return requested, true, nil
	}
	pkg, err := resolveBootPackage(requested)
	if err != nil {
		if errors.Is(err, errUntrustedAutomaticPackage) {
			appendBootWarningText(data, "ignored unsafe automatic package: "+bootAutoPkg)
			pkg = ""
		} else {
			return "", false, output.Fail(output.CodeStyBootstrapResourceMissing, err.Error()).WithData(data)
		}
	}
	return pkg, hasLocalRepo, nil
}

// prepareBootstrapLocale runs before even probing Ansible. A broken inherited
// locale can make ansible-playbook fail during process initialization, which
// otherwise looks indistinguishable from a missing Ansible installation.
func prepareBootstrapLocale(ctx context.Context, data *BootData) {
	if !needsDebianBootstrapLocale() {
		return
	}
	repaired, err := bootRepairLocale(ctx)
	if err != nil {
		data.Locale = "warning"
		appendBootWarning(data, "locale", err)
		return
	}
	if !repaired {
		data.Locale = "ready"
		writeBootStatus("locale", "ready")
		return
	}
	data.Locale = "repaired"
	data.Changed = true
	writeBootStatus("locale", "ready")
}

// retryBootstrapLocale re-attempts a locale repair that failed before package
// installation: on minimal Debian images /etc/locale.gen and locale-gen only
// exist after the locales package from the controller set is installed.
func retryBootstrapLocale(ctx context.Context, data *BootData) {
	if data.Locale != "warning" || !needsDebianBootstrapLocale() {
		return
	}
	repaired, err := bootRepairLocale(ctx)
	if err != nil {
		return // the first attempt already recorded the warning
	}
	filtered := data.Warnings[:0]
	for _, warning := range data.Warnings {
		if !strings.HasPrefix(warning, "locale: ") {
			filtered = append(filtered, warning)
		}
	}
	data.Warnings = filtered
	if repaired {
		data.Locale = "repaired"
		data.Changed = true
	} else {
		data.Locale = "ready"
	}
	writeBootStatus("locale", "ready")
}

// bootstrapAnsible installs the controller package set when the Ansible hard
// precheck fails. Offline repository preparation is shared with the ready fast
// path so an explicit or committed local repository is never ignored.
func bootstrapAnsible(opts NativeBootstrapOptions, data *BootData, pkg string, hasLocalRepo bool) *output.Result {
	overwrite := !opts.Keep
	manager := ext.PackageManagerCmd()
	if manager == "" {
		return output.Fail(output.CodeStyBootstrapResourceMissing, "native bootstrap supports dnf, yum on EL7, and apt-get").WithData(data)
	}
	if _, err := bootLookPath(manager); err != nil {
		return output.Fail(output.CodeStyBootstrapResourceMissing, fmt.Sprintf("package manager not found: %s", manager)).WithData(data)
	}
	data.PackageManager = manager
	if manager == "apt-get" {
		previous, existed := os.LookupEnv("DEBIAN_FRONTEND")
		_ = os.Setenv("DEBIAN_FRONTEND", "noninteractive")
		defer func() {
			if existed {
				_ = os.Setenv("DEBIAN_FRONTEND", previous)
			} else {
				_ = os.Unsetenv("DEBIAN_FRONTEND")
			}
		}()
	}

	if pkg != "" || hasLocalRepo {
		return prepareOfflineBootstrap(opts, data, pkg, hasLocalRepo, true)
	} else {
		region := strings.TrimSpace(opts.Region)
		result := bootRepos([]string{"node", "pigsty"}, repo.AddOptions{
			Region:   region,
			Mirror:   region == "china",
			Remove:   overwrite,
			Update:   true,
			GPGCheck: true,
			Quiet:    true,
		})
		recordBootRepositoryRollback(data, result)
		if result == nil {
			return output.Fail(output.CodeStyBootstrapFailed, "failed to configure Pigsty bootstrap repositories").WithData(data)
		}
		if !result.Success && (overwrite || result.Code != output.CodeRepoCacheUpdateFailed) {
			message := "failed to configure Pigsty bootstrap repositories"
			if result.Message != "" {
				message += ": " + result.Message
			}
			return output.Fail(output.CodeStyBootstrapFailed, message).WithDetail(result.Detail).WithData(data)
		}
		if missing := missingBootstrapRepoModules(result, "node", "pigsty"); len(missing) > 0 {
			return output.Fail(output.CodeStyBootstrapFailed,
				fmt.Sprintf("failed to configure required bootstrap repository modules: %s", strings.Join(missing, ", "))).WithData(data)
		}
		if !result.Success {
			data.Mode = "existing"
			appendBootWarningText(data, "Pigsty repository cache refresh failed; package installation will retry with all configured repositories")
		} else {
			data.Mode = "online"
		}
		if err := bootInstall(0, controllerBootstrapPackages(), install.PackageOptions{Yes: true, NoTranslation: true, Quiet: true}); err != nil {
			return failOnlineRepositoryReplacement(data, result, overwrite,
				fmt.Sprintf("failed to install controller packages: %v", err))
		}
	}
	return nil
}

func prepareOfflineBootstrap(opts NativeBootstrapOptions, data *BootData, pkg string, hasLocalRepo, installController bool) *output.Result {
	overwrite := !opts.Keep
	repoDir := bootRepoDir
	data.Mode, data.RepositoryDir = "offline", repoDir
	if !hasLocalRepo {
		data.PackagePath = pkg
		result := bootRepo(pkg, filepath.Dir(repoDir))
		if result == nil || !result.Success {
			message := "failed to unpack offline repository"
			if result != nil && result.Message != "" {
				message = result.Message
			}
			return output.Fail(output.CodeStyBootstrapFailed, message).WithData(data)
		}
		data.Changed = true
	} else if pkg != "" {
		appendBootWarningText(data, "committed local repository was reused; the selected offline package was not unpacked")
	}

	var backupInfo *repo.BackupInfo
	if overwrite {
		result := bootResetRepos(nil, false)
		recordBootRepositoryRollback(data, result)
		if result == nil || !result.Success {
			return output.Fail(output.CodeStyBootstrapFailed, "failed to back up existing repositories before configuring the local repository").WithData(data)
		}
		backupInfo = bootRepositoryBackupInfo(result)
		if backupInfo == nil {
			data.RepositoryRollback = "failed"
			appendBootWarningText(data, "repository backup completed without restoration metadata")
			return output.Fail(output.CodeStyBootstrapFailed, "repository backup metadata is unavailable").WithData(data)
		}
	}
	if err := bootAddLocal(repoDir); err != nil {
		return failOfflineRepositoryReplacement(data, backupInfo, overwrite,
			fmt.Sprintf("failed to configure local repository: %v", err))
	}
	data.Changed = true
	if installController {
		if err := bootInstall(0, controllerBootstrapPackages(), install.PackageOptions{Yes: true, NoTranslation: true, LocalOnly: true, Quiet: true}); err != nil {
			return failOfflineRepositoryReplacement(data, backupInfo, overwrite,
				fmt.Sprintf("failed to install controller packages from local repository: %v", err))
		}
	}
	return nil
}

func appendBootWarning(data *BootData, label string, err error) {
	if err == nil {
		return
	}
	appendBootWarningText(data, label+": "+conciseBootMessage(err))
}

func appendBootWarningText(data *BootData, warning string) {
	warning = conciseBootText(warning)
	if data == nil || warning == "" {
		return
	}
	data.Warnings = append(data.Warnings, warning)
}

func conciseBootMessage(err error) string {
	if err == nil {
		return ""
	}
	return conciseBootText(err.Error())
}

func conciseBootText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const limit = 240
	if runes := []rune(value); len(runes) > limit {
		return string(runes[:limit-3]) + "..."
	}
	return value
}

func writeBootOK(message string) {
	if config.IsStructuredOutput() || bootProgressOut == nil {
		return
	}
	style := configureTextStyle{color: output.IsColorEnabled()}
	fmt.Fprintln(bootProgressOut, style.ok(message))
}

func writeBootStatus(label, value string) {
	writeBootOK(fmt.Sprintf("%-7s = %s", label, value))
}

func displayPigstyStatus(status string) string {
	switch status {
	case pigstyInitExisting:
		return "exists"
	case pigstyInitInitialized, pigstyInitLocal:
		return "initialized"
	default:
		return status
	}
}

func recordBootRepositoryRollback(data *BootData, result *output.Result) {
	if data == nil || result == nil {
		return
	}
	rollback := bootRepositoryRollbackInfo(result)
	if rollback == nil {
		return
	}
	recordBootRollbackInfo(data, rollback)
}

func bootRepositoryRollbackInfo(result *output.Result) *repo.RollbackInfo {
	if result == nil {
		return nil
	}
	switch data := result.Data.(type) {
	case *repo.RepoAddData:
		if data != nil {
			return data.RollbackInfo
		}
	case repo.RepoAddData:
		return data.RollbackInfo
	case *repo.RepoRmData:
		if data != nil {
			return data.RollbackInfo
		}
	case repo.RepoRmData:
		return data.RollbackInfo
	}
	return nil
}

func bootRepositoryBackupInfo(result *output.Result) *repo.BackupInfo {
	if result == nil {
		return nil
	}
	switch data := result.Data.(type) {
	case *repo.RepoRmData:
		if data != nil {
			return data.BackupInfo
		}
	case repo.RepoRmData:
		return data.BackupInfo
	}
	return nil
}

func recordBootRollbackInfo(data *BootData, rollback *repo.RollbackInfo) {
	if data == nil || rollback == nil {
		return
	}
	if rollback.Success {
		data.RepositoryRollback = "restored"
		appendBootWarningText(data, "repository configuration failed; previous repository definitions were restored")
		return
	}
	data.RepositoryRollback = "failed"
	warning := "repository configuration failed and automatic rollback was incomplete"
	if rollback.Error != "" {
		warning += ": " + rollback.Error
	}
	appendBootWarningText(data, warning)
}

func failOfflineRepositoryReplacement(data *BootData, backup *repo.BackupInfo, overwrite bool, message string) *output.Result {
	if overwrite {
		rollback, err := bootRestoreRepos(backup, bootLocalRepo())
		if rollback == nil {
			rollback = &repo.RollbackInfo{}
		}
		if err != nil && rollback.Error == "" {
			rollback.Error = err.Error()
		}
		recordBootRollbackInfo(data, rollback)
	}
	return output.Fail(output.CodeStyBootstrapFailed, message).WithData(data)
}

// controllerBootstrapPackages mirrors the narrow responsibility of Pigsty's
// legacy bootstrap check_utils function and adds the controller-only Python and
// OpenSSH dependencies needed for verification and localhost repair.
// In particular, this must never reuse the much broader node-bootstrap alias:
// on Ubuntu that alias includes linux-tools-generic and can pull a new kernel.
func controllerBootstrapPackages() []string {
	switch config.OSType {
	case config.DistroDEB:
		packages := []string{"ansible", "python3", "python3-requests", "python3-jmespath", "python3-cryptography", "sow", "openssh-client", "sshpass", "unzip", "acl", "ca-certificates"}
		if needsDebianBootstrapLocale() {
			packages = append(packages, "locales")
		}
		return packages
	case config.DistroEL:
		switch config.OSVersion {
		case "7":
			return []string{"ansible", "python3", "python36-requests", "python36-idna", "sow", "openssh-clients", "sshpass", "unzip", "yum-utils", "ca-certificates"}
		case "8":
			return []string{"ansible", "python3.12-jmespath", "python3-cryptography", "openssh-clients", "sshpass", "unzip", "dnf-utils", "ca-certificates"}
		default:
			return []string{"ansible", "python3", "python3-requests", "python3-jmespath", "python3-cryptography", "sow", "openssh-clients", "sshpass", "unzip", "dnf-utils", "ca-certificates"}
		}
	default:
		return nil
	}
}

// bootRepoAddData unwraps the RepoAddData payload carried by a repository
// configuration Result, tolerating both pointer and value forms and typed nil.
func bootRepoAddData(result *output.Result) *repo.RepoAddData {
	if result == nil {
		return nil
	}
	switch value := result.Data.(type) {
	case *repo.RepoAddData:
		return value
	case repo.RepoAddData:
		return &value
	}
	return nil
}

// failOnlineRepositoryReplacement mirrors failOfflineRepositoryReplacement for
// the online branch: it restores the definitions bootRepos backed up and
// removes the repository files it wrote.
func failOnlineRepositoryReplacement(data *BootData, result *output.Result, overwrite bool, message string) *output.Result {
	if overwrite {
		var backup *repo.BackupInfo
		var added []string
		if repoData := bootRepoAddData(result); repoData != nil {
			backup = repoData.BackupInfo
			for _, item := range repoData.AddedRepos {
				if item != nil && item.FilePath != "" {
					added = append(added, item.FilePath)
				}
			}
		}
		rollback, err := bootRestoreRepos(backup, added...)
		if rollback == nil {
			rollback = &repo.RollbackInfo{}
		}
		if err != nil && rollback.Error == "" {
			rollback.Error = err.Error()
		}
		recordBootRollbackInfo(data, rollback)
	}
	return output.Fail(output.CodeStyBootstrapFailed, message).WithData(data)
}

func missingBootstrapRepoModules(result *output.Result, required ...string) []string {
	data := bootRepoAddData(result)
	if data == nil {
		return append([]string(nil), required...)
	}
	added := make(map[string]bool, len(data.AddedRepos))
	for _, item := range data.AddedRepos {
		if item != nil {
			added[item.Module] = true
		}
	}
	missing := make([]string, 0, len(required))
	for _, module := range required {
		if !added[module] {
			missing = append(missing, module)
		}
	}
	return missing
}

func resolveBootPackage(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		info, err := os.Lstat(bootAutoPkg)
		if os.IsNotExist(err) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect automatic offline package: %w", err)
		}
		if !info.Mode().IsRegular() || !trustedAutomaticPackage(info) {
			return "", fmt.Errorf("%w: %s", errUntrustedAutomaticPackage, bootAutoPkg)
		}
		return bootAutoPkg, nil
	}
	u, err := url.Parse(requested)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		if u.User != nil {
			return "", fmt.Errorf("offline package URL must not contain credentials")
		}
		digest := sha256.Sum256([]byte(requested))
		dst := filepath.Join("/tmp", "pigsty-pkg-"+hex.EncodeToString(digest[:6])+".tgz")
		if err := bootDownload(requested, dst); err != nil {
			return "", fmt.Errorf("download offline package: %w", err)
		}
		return dst, nil
	}
	abs, err := filepath.Abs(requested)
	if err != nil || !regularFile(abs) {
		return "", fmt.Errorf("offline package not found: %s", requested)
	}
	return abs, nil
}

func trustedAutomaticPackage(info os.FileInfo) bool {
	if info.Mode().Perm()&0o022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return false
	}
	if stat.Uid == 0 {
		return true
	}
	uid, err := strconv.ParseUint(strings.TrimSpace(os.Getenv("SUDO_UID")), 10, 32)
	return err == nil && stat.Uid == uint32(uid)
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

type ReexecOutcome struct {
	Output   []byte
	ExitCode int
	Err      error
	Started  bool
	// ResourceMissing marks failures raised while resolving bootstrap inputs
	// (offline package path or URL) before sudo was attempted, so callers can
	// report them as resource errors rather than privilege errors.
	ResourceMissing bool
}

// ReexecBootstrapAsRoot performs the one privilege transition needed by the
// Go extractor. Existing package/repository helpers still execute ordinary
// dnf/apt and file commands after the child is root.
func ReexecBootstrapAsRoot(args []string, stderr io.Writer) ReexecOutcome {
	if os.Getenv("PIG_NO_SUDO") == "1" || os.Getenv("PIG_NO_SUDO") == "true" {
		return ReexecOutcome{ExitCode: 3, Err: fmt.Errorf("PIG_NO_SUDO forbids sudo")}
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return ReexecOutcome{ExitCode: 3, Err: fmt.Errorf("sudo is not installed")}
	}
	executable, err := os.Executable()
	if err != nil {
		return ReexecOutcome{ExitCode: 3, Err: err}
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return ReexecOutcome{ExitCode: 3, Err: err}
	}
	args, err = prepareBootstrapArgs(args)
	if err != nil {
		return ReexecOutcome{ExitCode: 3, Err: err, ResourceMissing: true}
	}
	sudoArgs := []string{"--", executable}
	if os.Getenv("PIG_NON_INTERACTIVE") != "" {
		sudoArgs = append([]string{"-n"}, sudoArgs...)
	}
	childArgs := append(sudoArgs, args...)
	cmd := exec.Command("sudo", childArgs...)
	var stdout bytes.Buffer
	if config.IsStructuredOutput() {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = stderr
	err = cmd.Run()
	if err == nil {
		return ReexecOutcome{Output: stdout.Bytes(), Started: true}
	}
	started := cmd.ProcessState != nil
	exitCode := 3
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return ReexecOutcome{Output: stdout.Bytes(), ExitCode: exitCode, Err: err, Started: started}
}

func prepareBootstrapArgs(args []string) ([]string, error) {
	result := append([]string(nil), args...)
	if regularFile(filepath.Join(bootRepoDir, "repo_complete")) {
		return result, nil
	}
	for i := 0; i < len(result); i++ {
		if result[i] == "-p" || result[i] == "--path" {
			if i+1 >= len(result) {
				return nil, fmt.Errorf("%s requires a value", result[i])
			}
			prepared, err := resolveBootPackage(result[i+1])
			if err != nil {
				return nil, err
			}
			result[i+1] = prepared
			i++
			continue
		}
		if strings.HasPrefix(result[i], "--path=") {
			prepared, err := resolveBootPackage(strings.TrimPrefix(result[i], "--path="))
			if err != nil {
				return nil, err
			}
			result[i] = "--path=" + prepared
			continue
		}
		prefix, value, compact := compactBootstrapPathArg(result[i])
		if !compact {
			continue
		}
		if value == "" && !strings.HasSuffix(prefix, "=") {
			if i+1 >= len(result) {
				return nil, fmt.Errorf("%s requires a value", prefix)
			}
			prepared, err := resolveBootPackage(result[i+1])
			if err != nil {
				return nil, err
			}
			result[i+1] = prepared
			i++
			continue
		}
		prepared, err := resolveBootPackage(value)
		if err != nil {
			return nil, err
		}
		result[i] = prefix + prepared
	}
	return result, nil
}

// compactBootstrapPathArg recognizes pflag's compact shorthand forms such as
// -pFILE, -p=FILE, and grouped boolean shorthands like -mkpFILE.
func compactBootstrapPathArg(arg string) (prefix, value string, ok bool) {
	if len(arg) < 2 || arg[0] != '-' || strings.HasPrefix(arg, "--") {
		return "", "", false
	}
	body := arg[1:]
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case 'm', 'k':
			continue
		case 'p':
			prefix = "-" + body[:i+1]
			value = body[i+1:]
			if strings.HasPrefix(value, "=") {
				prefix += "="
				value = strings.TrimPrefix(value, "=")
			}
			return prefix, value, true
		default:
			return "", "", false
		}
	}
	return "", "", false
}
