package sty

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Mode               string   `json:"mode" yaml:"mode"`
	PackageManager     string   `json:"package_manager" yaml:"package_manager"`
	RepositoryPolicy   string   `json:"repository_policy" yaml:"repository_policy"`
	RepositoryRollback string   `json:"repository_rollback,omitempty" yaml:"repository_rollback,omitempty"`
	PackagePath        string   `json:"package_path,omitempty" yaml:"package_path,omitempty"`
	RepositoryDir      string   `json:"repository_dir,omitempty" yaml:"repository_dir,omitempty"`
	LocalSSH           string   `json:"local_ssh,omitempty" yaml:"local_ssh,omitempty"`
	PigstyHome         string   `json:"pigsty_home,omitempty" yaml:"pigsty_home,omitempty"`
	PigstyInit         string   `json:"pigsty_init,omitempty" yaml:"pigsty_init,omitempty"`
	Changed            bool     `json:"changed" yaml:"changed"`
	Warnings           []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

func (d *BootData) Text() string {
	if d == nil {
		return ""
	}
	var text strings.Builder
	fmt.Fprintf(&text, "  mode=%s manager=%s repos=%s changed=%t", d.Mode, d.PackageManager, d.RepositoryPolicy, d.Changed)
	if d.LocalSSH != "" {
		fmt.Fprintf(&text, " local_ssh=%s", d.LocalSSH)
	}
	if d.PigstyInit != "" {
		fmt.Fprintf(&text, " pigsty_init=%s", d.PigstyInit)
	}
	if d.RepositoryRollback != "" {
		fmt.Fprintf(&text, " repo_rollback=%s", d.RepositoryRollback)
	}
	for _, warning := range d.Warnings {
		fmt.Fprintf(&text, "\n  warning: %s", warning)
	}
	return text.String()
}

var (
	bootInstall      = install.InstallPackagesWithOptions
	bootRepos        = repo.ConfigureRepos
	bootResetRepos   = repo.RmRepos
	bootRepo         = repo.ExtractOfflineWithResult
	bootAddLocal     = repo.AddLocalRepo
	bootRestoreRepos = repo.RestoreRepos
	bootLocalRepo    = repo.LocalRepoPath
	bootLookPath     = exec.LookPath
	bootEUID         = os.Geteuid
	bootDownload     = utils.DownloadFile
	bootEnsureLocale = ensureDebianBootstrapLocale
	bootVerify       = verifyController
	bootEnsureSSH    = ensureLocalSSH
	bootEnsurePigsty = ensureDefaultPigsty
	bootAutoPkg      = "/tmp/pkg.tgz"
	bootRepoDir      = "/www/pigsty"
)

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
	data := &BootData{Kind: BootResultKind, RepositoryPolicy: policy}
	if bootEUID() != 0 {
		return output.Fail(output.CodeStyBootstrapPermissionDenied, "native bootstrap requires root privileges").WithData(data)
	}
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

	pkg, err := resolveBootPackage(opts.PackagePath)
	if err != nil {
		return output.Fail(output.CodeStyBootstrapResourceMissing, err.Error()).WithData(data)
	}
	repoDir := bootRepoDir
	hasLocalRepo := regularFile(filepath.Join(repoDir, "repo_complete"))
	if pkg != "" || hasLocalRepo {
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
		} else {
			if pkg != "" {
				data.Warnings = append(data.Warnings, "committed local repository was reused; the selected offline package was not unpacked")
			}
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
				data.Warnings = append(data.Warnings, "repository backup completed without restoration metadata")
				return output.Fail(output.CodeStyBootstrapFailed, "repository backup metadata is unavailable").WithData(data)
			}
		}
		if err := bootAddLocal(repoDir); err != nil {
			return failOfflineRepositoryReplacement(data, backupInfo, overwrite,
				fmt.Sprintf("failed to configure local repository: %v", err))
		}
		if err := bootInstall(0, controllerBootstrapPackages(), install.PackageOptions{Yes: true, NoTranslation: true, LocalOnly: true}); err != nil {
			return failOfflineRepositoryReplacement(data, backupInfo, overwrite,
				fmt.Sprintf("failed to install controller packages from local repository: %v", err))
		}
	} else {
		region := strings.TrimSpace(opts.Region)
		result := bootRepos([]string{"node", "pigsty"}, repo.AddOptions{
			Region:   region,
			Mirror:   region == "china",
			Remove:   overwrite,
			Update:   true,
			GPGCheck: true,
		})
		recordBootRepositoryRollback(data, result)
		if missing := missingBootstrapRepoModules(result, "node", "pigsty"); len(missing) > 0 {
			return output.Fail(output.CodeStyBootstrapFailed,
				fmt.Sprintf("failed to configure required bootstrap repository modules: %s", strings.Join(missing, ", "))).WithData(data)
		}
		if result == nil || !result.Success {
			if overwrite || result == nil || result.Code != output.CodeRepoCacheUpdateFailed {
				return output.Fail(output.CodeStyBootstrapFailed, "failed to configure Pigsty bootstrap repositories").WithData(data)
			}
			data.Mode = "existing"
			data.Warnings = append(data.Warnings, "Pigsty repository cache refresh failed; package installation will retry with all configured repositories")
		} else {
			data.Mode = "online"
		}
		if err := bootInstall(0, controllerBootstrapPackages(), install.PackageOptions{Yes: true, NoTranslation: true}); err != nil {
			return output.Fail(output.CodeStyBootstrapFailed, fmt.Sprintf("failed to install controller packages: %v", err)).WithData(data)
		}
	}

	if err := bootEnsureLocale(ctx); err != nil {
		return output.Fail(output.CodeStyBootstrapFailed, fmt.Sprintf("locale configuration failed: %v", err)).WithData(data)
	}
	if err := bootVerify(ctx); err != nil {
		return output.Fail(output.CodeStyBootstrapFailed, fmt.Sprintf("controller verification failed: %v", err)).WithData(data)
	}
	sshStatus, sshErr := bootEnsureSSH(ctx)
	data.LocalSSH = sshStatus
	if sshErr != nil {
		data.LocalSSH = "warning"
		data.Warnings = append(data.Warnings, fmt.Sprintf("localhost SSH is unavailable after best-effort repair: %v", sshErr))
	}
	pigsty, err := bootEnsurePigsty(ctx, opts.Region)
	data.PigstyHome = pigsty.Path
	data.PigstyInit = pigsty.Status
	if pigsty.Warning != "" {
		data.Warnings = append(data.Warnings, pigsty.Warning)
	}
	if err != nil {
		return output.Fail(output.CodeStyBootstrapFailed, fmt.Sprintf("failed to initialize Pigsty: %v", err)).
			WithData(data).
			WithNextActions(output.NextAction{Command: "pig sty init", Reason: "initialize the default Pigsty installation"})
	}
	data.Changed = true
	result := output.OK("Pigsty controller bootstrap complete", data)
	if pigsty.Status == pigstyInitUnavailable {
		result.WithNextActions(output.NextAction{Command: "pig sty init", Reason: "initialize Pigsty when its source repository is reachable"})
	} else {
		result.WithNextActions(output.NextAction{Command: "pig sty conf", Reason: "generate the Pigsty inventory"})
	}
	result.WithNextActions(output.NextAction{Command: "pig inventory check", Reason: "verify controller readiness"})
	return result
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
		data.Warnings = append(data.Warnings, "repository configuration failed; previous repository definitions were restored")
		return
	}
	data.RepositoryRollback = "failed"
	warning := "repository configuration failed and automatic rollback was incomplete"
	if rollback.Error != "" {
		warning += ": " + rollback.Error
	}
	data.Warnings = append(data.Warnings, warning)
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
			return []string{"ansible", "python3", "python3.12-jinja2", "python3.12-requests", "python3.12-jmespath", "python3.12-cryptography", "sow", "openssh-clients", "sshpass", "unzip", "dnf-utils", "ca-certificates"}
		default:
			return []string{"ansible", "python3", "python3-requests", "python3-jmespath", "python3-cryptography", "sow", "openssh-clients", "sshpass", "unzip", "dnf-utils", "ca-certificates"}
		}
	default:
		return nil
	}
}

func missingBootstrapRepoModules(result *output.Result, required ...string) []string {
	if result == nil {
		return append([]string(nil), required...)
	}
	var data *repo.RepoAddData
	switch value := result.Data.(type) {
	case *repo.RepoAddData:
		data = value
	case repo.RepoAddData:
		data = &value
	default:
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
			return "", fmt.Errorf("refusing untrusted automatic offline package %s; pass it explicitly with --path if intended", bootAutoPkg)
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
		return ReexecOutcome{ExitCode: 3, Err: err}
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
