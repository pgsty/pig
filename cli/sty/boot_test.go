package sty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pig/cli/install"
	"pig/cli/repo"
	"pig/internal/config"
	"pig/internal/output"
)

func setupBootTest(t *testing.T) {
	t.Helper()
	origInstall, origRepos, origReset, origRepo := bootInstall, bootRepos, bootResetRepos, bootRepo
	origAddLocal, origRestore, origLocalRepo, origLookPath := bootAddLocal, bootRestoreRepos, bootLocalRepo, bootLookPath
	origEUID, origLocale, origVerify, origEnsureSSH, origEnsurePigsty, origDownload := bootEUID, bootEnsureLocale, bootVerify, bootEnsureSSH, bootEnsurePigsty, bootDownload
	origAuto, origRepoDir := bootAutoPkg, bootRepoDir
	origType, origCode, origVendor, origVersion := config.OSType, config.OSCode, config.OSVendor, config.OSVersion
	t.Cleanup(func() {
		bootInstall, bootRepos, bootResetRepos, bootRepo = origInstall, origRepos, origReset, origRepo
		bootAddLocal, bootRestoreRepos, bootLocalRepo, bootLookPath = origAddLocal, origRestore, origLocalRepo, origLookPath
		bootEUID, bootEnsureLocale, bootVerify, bootEnsureSSH, bootEnsurePigsty, bootDownload = origEUID, origLocale, origVerify, origEnsureSSH, origEnsurePigsty, origDownload
		bootAutoPkg, bootRepoDir = origAuto, origRepoDir
		config.OSType, config.OSCode, config.OSVendor, config.OSVersion = origType, origCode, origVendor, origVersion
	})
	config.OSType, config.OSCode, config.OSVendor, config.OSVersion = config.DistroEL, "el10", "rocky", "10"
	bootEUID = func() int { return 0 }
	bootLookPath = func(string) (string, error) { return "/usr/bin/dnf", nil }
	bootEnsureLocale = func(context.Context) error { return nil }
	bootEnsureSSH = func(context.Context) (string, error) { return "ok", nil }
	bootEnsurePigsty = func(context.Context, string) (pigstyInitOutcome, error) {
		return pigstyInitOutcome{Path: "/home/admin/pigsty", Status: pigstyInitExisting}, nil
	}
	bootAutoPkg = filepath.Join(t.TempDir(), "missing.tgz")
	bootRepoDir = filepath.Join(t.TempDir(), "pigsty")
	bootResetRepos = func([]string, bool) *output.Result {
		return output.OK("backed up", &repo.RepoRmData{BackupInfo: &repo.BackupInfo{BackupDir: "/fixture/repo-backup"}})
	}
	bootAddLocal = func(string) error { return nil }
	bootLocalRepo = func() string { return "/fixture/local.repo" }
}

func TestBootstrapNativeReadyStillConvergesRepositoriesAndPackages(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	repoCalls, installCalls := 0, 0
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		repoCalls++
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || !opts.Remove || !opts.Update || !opts.GPGCheck {
			t.Fatalf("bootstrap repo request = %v %+v", modules, opts)
		}
		return successfulBootstrapRepoResult()
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		installCalls++
		return nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || !data.Changed || data.Mode != "online" || data.RepositoryPolicy != "overwrite" || repoCalls != 1 || installCalls != 1 {
		t.Fatalf("unexpected ready result: %+v %+v", result, data)
	}
}

func TestBootstrapNativeRequiresRootWithPermissionCode(t *testing.T) {
	setupBootTest(t)
	bootEUID = func() int { return 501 }
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	if result.Success || result.Code != output.CodeStyBootstrapPermissionDenied {
		t.Fatalf("non-root native Bootstrap result = %+v", result)
	}
}

func TestBootstrapNativeKeepPreservesOnlineRepos(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	bootInstall = func(_ int, _ []string, _ install.PackageOptions) error { return nil }
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || opts.Remove || !opts.Update || !opts.GPGCheck {
			t.Fatalf("keep repo request = %v %+v", modules, opts)
		}
		return successfulBootstrapRepoResult()
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{Keep: true})
	data := result.Data.(*BootData)
	if !result.Success || data.Mode != "online" || data.RepositoryPolicy != "keep" {
		t.Fatalf("unexpected keep result: %+v %+v", result, data)
	}
}

func TestBootstrapNativeOfflineUsesStrictLocalScope(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := writeTinyFile(pkg); err != nil {
		t.Fatal(err)
	}
	bootRepo = func(gotPkg, target string) *output.Result {
		if gotPkg != pkg || target != filepath.Dir(bootRepoDir) {
			t.Fatalf("offline repo args = %q %q", gotPkg, target)
		}
		return output.OK("ready", nil)
	}
	installCalls := 0
	bootInstall = func(_ int, names []string, opts install.PackageOptions) error {
		installCalls++
		if !opts.LocalOnly || !opts.NoTranslation || !containsPackage(names, "sow") ||
			!containsPackage(names, "ca-certificates") || containsPackage(names, "tar") || containsPackage(names, "gzip") {
			t.Fatalf("offline install %d = %v %+v", installCalls, names, opts)
		}
		return nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{PackagePath: pkg})
	data := result.Data.(*BootData)
	if !result.Success || data.Mode != "offline" || !data.Changed || installCalls != 1 {
		t.Fatalf("unexpected offline result: %+v %+v", result, data)
	}
}

func TestBootstrapNativeOfflineOrdersExtractBackupAndLocalDefinition(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := writeTinyFile(pkg); err != nil {
		t.Fatal(err)
	}
	order := make([]string, 0, 4)
	bootRepo = func(string, string) *output.Result {
		order = append(order, "extract")
		return output.OK("extracted", nil)
	}
	backup := &repo.BackupInfo{BackupDir: "/fixture/repo-backup"}
	bootResetRepos = func([]string, bool) *output.Result {
		order = append(order, "backup")
		return output.OK("backed up", &repo.RepoRmData{BackupInfo: backup})
	}
	bootAddLocal = func(string) error {
		order = append(order, "local")
		return nil
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		order = append(order, "install")
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{PackagePath: pkg})
	if !result.Success || strings.Join(order, ",") != "extract,backup,local,install" {
		t.Fatalf("offline replacement order = %v, result=%+v", order, result)
	}
}

func TestBootstrapNativeOfflineRestoresReposAfterLocalInstallFailure(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	backup := &repo.BackupInfo{BackupDir: "/fixture/repo-backup", BackedUpFiles: []string{"/fixture/original.repo"}}
	bootResetRepos = func([]string, bool) *output.Result {
		return output.OK("backed up", &repo.RepoRmData{BackupInfo: backup})
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		return errors.New("local cache failed")
	}
	restored := false
	bootRestoreRepos = func(got *repo.BackupInfo, added ...string) (*repo.RollbackInfo, error) {
		restored = true
		if got != backup || len(added) != 1 || added[0] != "/fixture/local.repo" {
			t.Fatalf("offline restore args = %+v %v", got, added)
		}
		return &repo.RollbackInfo{Success: true, RestoredFiles: backup.BackedUpFiles}, nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if result.Success || !restored || data.RepositoryRollback != "restored" ||
		!strings.Contains(result.Message, "local cache failed") {
		t.Fatalf("offline install rollback = %+v %+v restored=%t", result, data, restored)
	}
}

func TestBootstrapNativeReportsSelectedPackageWhenCommittedRepoIsReused(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "new-pkg.tgz")
	if err := writeTinyFile(pkg); err != nil {
		t.Fatal(err)
	}
	bootRepo = func(string, string) *output.Result {
		t.Fatal("committed repository should be reused without unpacking")
		return nil
	}
	bootAddLocal = func(got string) error {
		if got != bootRepoDir {
			t.Fatalf("local repo path = %q", got)
		}
		return nil
	}
	installCalls := 0
	bootInstall = func(_ int, names []string, opts install.PackageOptions) error {
		installCalls++
		if !opts.LocalOnly || !containsPackage(names, "ansible") {
			t.Fatalf("committed local repository install = %v %+v", names, opts)
		}
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{PackagePath: pkg})
	data := result.Data.(*BootData)
	if !result.Success || data.PackagePath != "" || installCalls != 1 || len(data.Warnings) != 1 ||
		!strings.Contains(data.Warnings[0], "was not unpacked") {
		t.Fatalf("unexpected committed-repo result: %+v %+v", result, data)
	}
}

func TestResolveBootPackageProtectsAutomaticTmpPackage(t *testing.T) {
	setupBootTest(t)
	if err := writeTinyFile(bootAutoPkg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUDO_UID", strconv.Itoa(os.Getuid()))
	got, err := resolveBootPackage("")
	if err != nil || got != bootAutoPkg {
		t.Fatalf("trusted automatic package = %q, %v", got, err)
	}
	if err := os.Chmod(bootAutoPkg, 0o622); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBootPackage(""); err == nil {
		t.Fatal("group/world-writable automatic package was accepted")
	}
}

func TestPrepareBootstrapArgsResolvesCompactPathBeforeSudo(t *testing.T) {
	setupBootTest(t)
	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := writeTinyFile(pkg); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, arg, prefix string
	}{
		{name: "compact", arg: "-p" + pkg, prefix: "-p"},
		{name: "equals", arg: "-p=" + pkg, prefix: "-p="},
		{name: "grouped", arg: "-mkp" + pkg, prefix: "-mkp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prepareBootstrapArgs([]string{"sty", "boot", tc.arg})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 3 || got[2] != tc.prefix+pkg {
				t.Fatalf("prepared args = %v, want path prefix %q", got, tc.prefix+pkg)
			}
		})
	}
}

func TestBootstrapNativeAddsPigstyReposBeforeControllerPackages(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	var requests [][]string
	reposAdded := false
	bootInstall = func(_ int, names []string, _ install.PackageOptions) error {
		if containsPackage(names, "sow") && !reposAdded {
			t.Fatal("controller packages were installed before Pigsty repositories")
		}
		requests = append(requests, append([]string(nil), names...))
		return nil
	}
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || !opts.Remove || !opts.Update || !opts.GPGCheck {
			t.Fatalf("bootstrap repo request = %v %+v", modules, opts)
		}
		reposAdded = true
		return successfulBootstrapRepoResult()
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.Mode != "online" || !reposAdded || len(requests) != 1 || !containsPackage(requests[0], "sow") {
		t.Fatalf("unexpected existing result: %+v %+v requests=%v", result, data, requests)
	}
}

func TestBootstrapNativeDefaultsToReplacingOnlineRepos(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	bootInstall = func(_ int, _ []string, _ install.PackageOptions) error { return nil }
	reposCalled := 0
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		reposCalled++
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || !opts.Remove || !opts.Update || !opts.GPGCheck {
			t.Fatalf("overwrite repo request = %v %+v", modules, opts)
		}
		return successfulBootstrapRepoResult()
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{Region: "china"})
	data := result.Data.(*BootData)
	if !result.Success || data.Mode != "online" || data.RepositoryPolicy != "overwrite" || reposCalled != 1 {
		t.Fatalf("unexpected overwrite result: %+v %+v calls=%d", result, data, reposCalled)
	}
}

func TestBootstrapNativeDefaultsToResettingOfflineRepos(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	reset, local := false, false
	bootResetRepos = func(modules []string, update bool) *output.Result {
		if len(modules) != 0 || update {
			t.Fatalf("offline reset args = %v %t", modules, update)
		}
		reset = true
		return output.OK("backed up", &repo.RepoRmData{BackupInfo: &repo.BackupInfo{BackupDir: "/fixture/repo-backup"}})
	}
	bootAddLocal = func(string) error {
		if !reset {
			t.Fatal("local repository configured before existing repositories were reset")
		}
		local = true
		return nil
	}
	bootInstall = func(_ int, _ []string, _ install.PackageOptions) error { return nil }

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	if !result.Success || !reset || !local {
		t.Fatalf("unexpected default offline result: %+v reset=%t local=%t", result, reset, local)
	}
}

func TestBootstrapNativeKeepPreservesOfflineRepos(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	bootResetRepos = func([]string, bool) *output.Result {
		t.Fatal("--keep reset existing repositories")
		return nil
	}
	local := false
	bootAddLocal = func(string) error {
		local = true
		return nil
	}
	bootInstall = func(_ int, _ []string, opts install.PackageOptions) error {
		if !opts.LocalOnly {
			t.Fatalf("offline keep install was not local-only: %+v", opts)
		}
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{Keep: true})
	data := result.Data.(*BootData)
	if !result.Success || !local || data.RepositoryPolicy != "keep" {
		t.Fatalf("unexpected offline keep result: %+v %+v local=%t", result, data, local)
	}
}

func TestBootstrapNativeDefaultCacheFailureIsFatal(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	bootRepos = func([]string, repo.AddOptions) *output.Result {
		return output.Fail(output.CodeRepoCacheUpdateFailed, "cache update failed").WithData(&repo.RepoAddData{
			AddedRepos:   []*repo.AddedRepoItem{{Module: "node"}, {Module: "pigsty"}},
			RollbackInfo: &repo.RollbackInfo{Success: true},
		})
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		t.Fatal("default overwrite continued after repository cache failure")
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if result.Success || !strings.Contains(result.Message, "failed to configure") ||
		data.RepositoryRollback != "restored" || len(data.Warnings) != 1 {
		t.Fatalf("default cache failure result = %+v", result)
	}
}

func TestBootstrapNativeKeepCanFallBackToExistingRepos(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	bootRepos = func(_ []string, opts repo.AddOptions) *output.Result {
		if opts.Remove {
			t.Fatal("--keep requested repository removal")
		}
		return failedBootstrapRepoCacheResult()
	}
	installed := false
	bootInstall = func(int, []string, install.PackageOptions) error {
		installed = true
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{Keep: true})
	data := result.Data.(*BootData)
	if !result.Success || !installed || data.Mode != "existing" || data.RepositoryPolicy != "keep" || len(data.Warnings) != 1 {
		t.Fatalf("keep fallback result = %+v %+v installed=%t", result, data, installed)
	}
}

func TestBootstrapNativeRejectsPartialBootstrapRepositories(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	bootRepos = func([]string, repo.AddOptions) *output.Result {
		return output.OK("partially added", &repo.RepoAddData{
			AddedRepos: []*repo.AddedRepoItem{{Module: "node"}},
			Failed:     []*repo.FailedRepoItem{{Module: "pigsty", Error: "write failed"}},
		})
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		t.Fatal("partial repository set reached package installation")
		return nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	if result.Success || !strings.Contains(result.Message, "pigsty") {
		t.Fatalf("partial repository result = %+v", result)
	}
}

func TestMissingBootstrapRepoModulesFailsClosedWithoutDetails(t *testing.T) {
	missing := missingBootstrapRepoModules(output.OK("added", nil), "node", "pigsty")
	if len(missing) != 2 || missing[0] != "node" || missing[1] != "pigsty" {
		t.Fatalf("missing repository details were accepted: %v", missing)
	}
}

func TestBootstrapNativeLocalSSHFailureIsWarning(t *testing.T) {
	setupBootTest(t)
	bootVerify = func(context.Context) error { return nil }
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootEnsureSSH = func(context.Context) (string, error) {
		return "warning", errors.New("sshd is unavailable")
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.LocalSSH != "warning" || len(data.Warnings) != 1 {
		t.Fatalf("non-fatal SSH result = %+v %+v", result, data)
	}
	if text := data.Text(); !strings.Contains(text, "warning: localhost SSH") {
		t.Fatalf("text output omitted SSH warning: %q", text)
	}
}

func TestBootstrapNativeConfiguresLocaleAfterPackages(t *testing.T) {
	setupBootTest(t)
	order := make([]string, 0, 3)
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error {
		order = append(order, "install")
		return nil
	}
	bootEnsureLocale = func(context.Context) error {
		order = append(order, "locale")
		return nil
	}
	bootVerify = func(context.Context) error {
		order = append(order, "verify")
		return nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	if !result.Success || strings.Join(order, ",") != "install,locale,verify" {
		t.Fatalf("bootstrap order = %v, result=%+v", order, result)
	}
}

func TestBootstrapNativeLocaleFailureIsFatal(t *testing.T) {
	setupBootTest(t)
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootEnsureLocale = func(context.Context) error { return errors.New("locale-gen failed") }
	bootVerify = func(context.Context) error {
		t.Fatal("controller verification ran after locale failure")
		return nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	if result.Success || !strings.Contains(result.Message, "locale configuration failed") {
		t.Fatalf("locale failure result = %+v", result)
	}
}

func TestBootstrapNativeInitializesMissingDefaultPigsty(t *testing.T) {
	setupBootTest(t)
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootVerify = func(context.Context) error { return nil }
	bootEnsurePigsty = func(_ context.Context, region string) (pigstyInitOutcome, error) {
		if region != "china" {
			t.Fatalf("auto-init region = %q", region)
		}
		return pigstyInitOutcome{Path: "/home/admin/pigsty", Status: pigstyInitInitialized}, nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{Region: "china"})
	data := result.Data.(*BootData)
	if !result.Success || data.PigstyHome != "/home/admin/pigsty" || data.PigstyInit != pigstyInitInitialized {
		t.Fatalf("auto-init result = %+v %+v", result, data)
	}
	if text := data.Text(); !strings.Contains(text, "pigsty_init=initialized") {
		t.Fatalf("auto-init status missing from text: %q", text)
	}
}

func TestBootstrapNativePigstyInitFailureIsFatal(t *testing.T) {
	setupBootTest(t)
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootVerify = func(context.Context) error { return nil }
	bootEnsurePigsty = func(context.Context, string) (pigstyInitOutcome, error) {
		return pigstyInitOutcome{Path: "/home/admin/pigsty", Status: pigstyInitFailed}, errors.New("download failed")
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if result.Success || data.PigstyInit != pigstyInitFailed || !strings.Contains(result.Message, "failed to initialize Pigsty") {
		t.Fatalf("auto-init failure result = %+v %+v", result, data)
	}
}

func TestBootstrapNativeMissingPigstyWithoutNetworkWarns(t *testing.T) {
	setupBootTest(t)
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootVerify = func(context.Context) error { return nil }
	bootEnsurePigsty = func(context.Context, string) (pigstyInitOutcome, error) {
		return pigstyInitOutcome{
			Path:    "/home/admin/pigsty",
			Status:  pigstyInitUnavailable,
			Warning: "Pigsty source repository is unreachable",
		}, nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.PigstyInit != pigstyInitUnavailable || len(data.Warnings) != 1 {
		t.Fatalf("unavailable auto-init result = %+v %+v", result, data)
	}
	if len(result.NextActions) < 1 || result.NextActions[0].Command != "pig sty init" {
		t.Fatalf("unavailable auto-init next actions = %+v", result.NextActions)
	}
	for _, action := range result.NextActions {
		if action.Command == "pig sty conf" {
			t.Fatalf("configure was suggested before Pigsty initialization: %+v", result.NextActions)
		}
	}
}

func TestControllerBootstrapPackagesStayLean(t *testing.T) {
	origType, origVendor, origVersion := config.OSType, config.OSVendor, config.OSVersion
	t.Cleanup(func() { config.OSType, config.OSVendor, config.OSVersion = origType, origVendor, origVersion })
	for _, tc := range []struct {
		name, osType, vendor, version string
		wantLocales                   bool
	}{
		{name: "debian12", osType: config.DistroDEB, vendor: "debian", version: "12", wantLocales: true},
		{name: "debian13", osType: config.DistroDEB, vendor: "debian", version: "13", wantLocales: true},
		{name: "ubuntu", osType: config.DistroDEB, vendor: "ubuntu", version: "26"},
		{name: "el8", osType: config.DistroEL, vendor: "rocky", version: "8"},
		{name: "el10", osType: config.DistroEL, vendor: "rocky", version: "10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.OSType, config.OSVendor, config.OSVersion = tc.osType, tc.vendor, tc.version
			packages := controllerBootstrapPackages()
			cryptoPackage := "python3-cryptography"
			if tc.osType == config.DistroEL && tc.version == "8" {
				cryptoPackage = "python3.12-cryptography"
				for _, required := range []string{"python3.12-jinja2", "python3.12-requests", "python3.12-jmespath"} {
					if !containsPackage(packages, required) {
						t.Fatalf("EL8 Ansible runtime package %q missing: %v", required, packages)
					}
				}
			}
			if !containsPackage(packages, "sow") || !containsPackage(packages, "ansible") ||
				!containsPackage(packages, "ca-certificates") || !containsPackage(packages, cryptoPackage) {
				t.Fatalf("required controller packages missing: %v", packages)
			}
			sshClient := "openssh-clients"
			if tc.osType == config.DistroDEB {
				sshClient = "openssh-client"
			}
			if !containsPackage(packages, sshClient) {
				t.Fatalf("SSH client package missing: %v", packages)
			}
			if containsPackage(packages, "locales") != tc.wantLocales {
				t.Fatalf("locales package mismatch: want=%t packages=%v", tc.wantLocales, packages)
			}
			for _, pkg := range packages {
				if strings.HasPrefix(pkg, "linux-") || pkg == "perf" || strings.Contains(pkg, "ftp") || pkg == "tar" || pkg == "gzip" {
					t.Fatalf("node/kernel package leaked into controller bootstrap: %q in %v", pkg, packages)
				}
			}
		})
	}
}

func containsPackage(packages []string, want string) bool {
	for _, pkg := range packages {
		if pkg == want {
			return true
		}
	}
	return false
}

func successfulBootstrapRepoResult() *output.Result {
	return output.OK("added", &repo.RepoAddData{AddedRepos: []*repo.AddedRepoItem{
		{Module: "node"},
		{Module: "pigsty"},
	}})
}

func failedBootstrapRepoCacheResult() *output.Result {
	return output.Fail(output.CodeRepoCacheUpdateFailed, "cache update failed").WithData(&repo.RepoAddData{AddedRepos: []*repo.AddedRepoItem{
		{Module: "node"},
		{Module: "pigsty"},
	}})
}

func writeTinyFile(path string) error {
	return os.WriteFile(path, []byte("fixture"), 0o600)
}
