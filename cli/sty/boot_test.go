package sty

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	origEUID, origRepairLocale, origVerify, origVerifyHelpers := bootEUID, bootRepairLocale, bootVerifyAnsible, bootVerifyHelpers
	origEnsureSSH, origEnsurePigsty, origDownload := bootEnsureSSH, bootEnsurePigsty, bootDownload
	origAuto, origRepoDir, origProgress := bootAutoPkg, bootRepoDir, bootProgressOut
	origType, origCode, origVendor, origVersion := config.OSType, config.OSCode, config.OSVendor, config.OSVersion
	t.Cleanup(func() {
		bootInstall, bootRepos, bootResetRepos, bootRepo = origInstall, origRepos, origReset, origRepo
		bootAddLocal, bootRestoreRepos, bootLocalRepo, bootLookPath = origAddLocal, origRestore, origLocalRepo, origLookPath
		bootEUID, bootRepairLocale, bootVerifyAnsible, bootVerifyHelpers = origEUID, origRepairLocale, origVerify, origVerifyHelpers
		bootEnsureSSH, bootEnsurePigsty, bootDownload = origEnsureSSH, origEnsurePigsty, origDownload
		bootAutoPkg, bootRepoDir, bootProgressOut = origAuto, origRepoDir, origProgress
		config.OSType, config.OSCode, config.OSVendor, config.OSVersion = origType, origCode, origVendor, origVersion
	})
	config.OSType, config.OSCode, config.OSVendor, config.OSVersion = config.DistroEL, "el10", "rocky", "10"
	bootEUID = func() int { return 0 }
	bootLookPath = func(string) (string, error) { return "/usr/bin/dnf", nil }
	verifyCalls := 0
	bootVerifyAnsible = func(context.Context) error {
		verifyCalls++
		if verifyCalls == 1 {
			return errors.New("ansible unavailable")
		}
		return nil
	}
	bootRepairLocale = func(context.Context) (bool, error) { return false, nil }
	bootVerifyHelpers = func(context.Context) error { return nil }
	bootEnsureSSH = func(context.Context) (string, error) { return "ok", nil }
	bootEnsurePigsty = func(context.Context, string) (pigstyInitOutcome, error) {
		return pigstyInitOutcome{Path: "/home/admin/pigsty", Status: pigstyInitExisting}, nil
	}
	bootAutoPkg = filepath.Join(t.TempDir(), "missing.tgz")
	bootRepoDir = filepath.Join(t.TempDir(), "pigsty")
	bootProgressOut = io.Discard
	bootResetRepos = func([]string, bool) *output.Result {
		return output.OK("backed up", &repo.RepoRmData{BackupInfo: &repo.BackupInfo{BackupDir: "/fixture/repo-backup"}})
	}
	bootAddLocal = func(string) error { return nil }
	bootLocalRepo = func() string { return "/fixture/local.repo" }
}

func TestBootOKUsesLegacyStyleAndSuppressesStructuredOutput(t *testing.T) {
	origOut, origFormat := bootProgressOut, config.OutputFormat
	t.Cleanup(func() {
		bootProgressOut, config.OutputFormat = origOut, origFormat
	})
	t.Setenv("NO_COLOR", "1")
	var buffer bytes.Buffer
	bootProgressOut = &buffer
	config.OutputFormat = config.OUTPUT_TEXT
	writeBootStatus("ssh", "ok")
	if got := buffer.String(); got != "[ OK ] ssh     = ok\n" {
		t.Fatalf("Bootstrap progress = %q", got)
	}

	buffer.Reset()
	config.OutputFormat = config.OUTPUT_JSON
	writeBootOK("hidden")
	if buffer.Len() != 0 {
		t.Fatalf("structured Bootstrap emitted progress: %q", buffer.String())
	}
}

func TestBootstrapNativeReadyWithoutOfflineSourceSkipsRepositoryAndPackageBootstrap(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return nil }
	repoCalls, installCalls := 0, 0
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		repoCalls++
		t.Fatalf("ready Ansible configured repositories: %v %+v", modules, opts)
		return nil
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		installCalls++
		return nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.Changed || data.Ansible != "ready" || data.Mode != "ready" || repoCalls != 0 || installCalls != 0 {
		t.Fatalf("unexpected ready result: %+v %+v", result, data)
	}
}

func TestBootstrapNativeReadyPreparesExplicitOfflineRepositoryWithoutInstalling(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return nil }
	pkg := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := writeTinyFile(pkg); err != nil {
		t.Fatal(err)
	}
	order := make([]string, 0, 3)
	bootRepo = func(gotPkg, target string) *output.Result {
		order = append(order, "extract")
		if gotPkg != pkg || target != filepath.Dir(bootRepoDir) {
			t.Fatalf("offline repo args = %q %q", gotPkg, target)
		}
		return output.OK("extracted", nil)
	}
	bootResetRepos = func([]string, bool) *output.Result {
		order = append(order, "backup")
		return output.OK("backed up", &repo.RepoRmData{BackupInfo: &repo.BackupInfo{BackupDir: "/fixture/repo-backup"}})
	}
	bootAddLocal = func(got string) error {
		order = append(order, "local")
		if got != bootRepoDir {
			t.Fatalf("local repo path = %q", got)
		}
		return nil
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		t.Fatal("ready Ansible reinstalled controller packages")
		return nil
	}
	bootRepos = func([]string, repo.AddOptions) *output.Result {
		t.Fatal("explicit offline package configured online repositories")
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{PackagePath: pkg})
	data := result.Data.(*BootData)
	if !result.Success || data.Ansible != "ready" || data.Mode != "offline" || data.PackagePath != pkg ||
		!data.Changed || strings.Join(order, ",") != "extract,backup,local" {
		t.Fatalf("ready offline result = %+v data=%+v order=%v", result, data, order)
	}
}

func TestBootstrapNativeReadyReusesCommittedOfflineRepository(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return nil }
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(t.TempDir(), "already-unneeded.tgz")
	bootRepo = func(string, string) *output.Result {
		t.Fatal("committed repository was unpacked again")
		return nil
	}
	bootInstall = func(int, []string, install.PackageOptions) error {
		t.Fatal("ready Ansible reinstalled controller packages")
		return nil
	}
	localCalls := 0
	bootAddLocal = func(got string) error {
		localCalls++
		if got != bootRepoDir {
			t.Fatalf("local repo path = %q", got)
		}
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{PackagePath: pkg})
	data := result.Data.(*BootData)
	if !result.Success || data.Ansible != "ready" || data.Mode != "offline" || data.PackagePath != "" ||
		localCalls != 1 || len(data.Warnings) != 1 || !strings.Contains(data.Warnings[0], "was not unpacked") {
		t.Fatalf("ready committed-repo result = %+v data=%+v calls=%d", result, data, localCalls)
	}
}

func TestBootstrapNativeReadyRejectsMissingExplicitOfflinePackage(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return nil }
	missing := filepath.Join(t.TempDir(), "missing.tgz")
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{PackagePath: missing})
	data := result.Data.(*BootData)
	if result.Success || data.Ansible != "ready" || result.Code != output.CodeStyBootstrapResourceMissing ||
		!strings.Contains(result.Message, "offline package not found") {
		t.Fatalf("missing explicit package result = %+v data=%+v", result, data)
	}
}

func TestBootstrapNativeReadyOutputReportsResultsOnly(t *testing.T) {
	setupBootTest(t)
	config.OSType, config.OSCode, config.OSVendor, config.OSVersion = config.DistroDEB, "d12", "debian", "12"
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	t.Setenv("NO_COLOR", "1")
	config.OutputFormat = config.OUTPUT_TEXT
	var progress bytes.Buffer
	bootProgressOut = &progress
	bootVerifyAnsible = func(context.Context) error { return nil }

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	gotProgress := progress.String()
	for _, unwanted := range []string{"[HINT]", "checking"} {
		if strings.Contains(gotProgress, unwanted) {
			t.Fatalf("quiet Bootstrap progress contains %q: %q", unwanted, gotProgress)
		}
	}
	wantProgress := "[ OK ] locale  = ready\n" +
		"[ OK ] ansible = ready\n" +
		"[ OK ] ssh     = ok\n" +
		"[ OK ] pigsty  = exists\n" +
		"[ OK ] bootstrap complete\n"
	if gotProgress != wantProgress {
		t.Fatalf("quiet Bootstrap progress:\nwant: %q\n got: %q", wantProgress, gotProgress)
	}

	text := result.Data.(*BootData).Text()
	wantText := "proceed with:\n" +
		"    pig sty conf -g   # generate config with random passwords\n" +
		"    pig inv edit      # edit the inventory, [OPTIONAL]\n" +
		"    pig sty deploy    # run the actual deployment"
	if text != wantText {
		t.Fatalf("Bootstrap next steps:\nwant: %q\n got: %q", wantText, text)
	}
}

func TestBootstrapNativeFailsWhenAnsibleIsStillUnavailableAfterBootstrap(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return errors.New("ansible-playbook unavailable") }
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if result.Success || data.Ansible != "failed" || !strings.Contains(result.Message, "ansible bootstrap verification failed") {
		t.Fatalf("final Ansible verification result = %+v %+v", result, data)
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
	bootInstall = func(_ int, _ []string, _ install.PackageOptions) error { return nil }
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || opts.Remove || !opts.Update || !opts.GPGCheck || !opts.Quiet {
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
		if !opts.LocalOnly || !opts.NoTranslation || !opts.Quiet || !containsPackage(names, "sow") ||
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

func TestBootstrapNativeIgnoresUnsafeAutomaticPackageAndUsesCommittedRepo(t *testing.T) {
	setupBootTest(t)
	if err := writeTinyFile(bootAutoPkg); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bootAutoPkg, 0o622); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	bootRepo = func(string, string) *output.Result {
		t.Fatal("unsafe automatic package was unpacked")
		return nil
	}
	bootInstall = func(_ int, _ []string, opts install.PackageOptions) error {
		if !opts.LocalOnly || !opts.Quiet {
			t.Fatalf("committed repository install options = %+v", opts)
		}
		return nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.Mode != "offline" || data.PackagePath != "" || len(data.Warnings) != 1 ||
		!strings.Contains(data.Warnings[0], "ignored unsafe") {
		t.Fatalf("unsafe automatic package result = %+v %+v", result, data)
	}
	if _, err := os.Stat(bootAutoPkg); err != nil {
		t.Fatalf("unsafe automatic package was removed: %v", err)
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

func TestPrepareBootstrapArgsSkipsSelectedPackageWhenCommittedRepoExists(t *testing.T) {
	setupBootTest(t)
	if err := os.MkdirAll(bootRepoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTinyFile(filepath.Join(bootRepoDir, "repo_complete")); err != nil {
		t.Fatal(err)
	}
	bootDownload = func(string, string) error {
		t.Fatal("selected package was downloaded despite a committed repository")
		return nil
	}
	args := []string{"sty", "boot", "--path", "https://example.com/pkg.tgz"}
	got, err := prepareBootstrapArgs(args)
	if err != nil || strings.Join(got, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("prepared committed-repo args = %v, %v; want %v", got, err, args)
	}
}

func TestBootstrapNativeAddsPigstyReposBeforeControllerPackages(t *testing.T) {
	setupBootTest(t)
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
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || !opts.Remove || !opts.Update || !opts.GPGCheck || !opts.Quiet {
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
	bootInstall = func(_ int, _ []string, _ install.PackageOptions) error { return nil }
	reposCalled := 0
	bootRepos = func(modules []string, opts repo.AddOptions) *output.Result {
		reposCalled++
		if len(modules) != 2 || modules[0] != "node" || modules[1] != "pigsty" || !opts.Remove || !opts.Update || !opts.GPGCheck || !opts.Quiet {
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
	if text := data.Text(); !strings.Contains(text, "[WARN] localhost SSH") {
		t.Fatalf("text output omitted SSH warning: %q", text)
	}
}

func TestBootstrapNativePreparesLocaleBeforeAnsibleAndSSH(t *testing.T) {
	setupBootTest(t)
	config.OSType, config.OSCode, config.OSVendor, config.OSVersion = config.DistroDEB, "d12", "debian", "12"
	origFormat := config.OutputFormat
	t.Cleanup(func() { config.OutputFormat = origFormat })
	t.Setenv("NO_COLOR", "1")
	config.OutputFormat = config.OUTPUT_TEXT
	var progress bytes.Buffer
	bootProgressOut = &progress
	order := make([]string, 0, 3)
	bootRepairLocale = func(context.Context) (bool, error) {
		order = append(order, "locale")
		return true, nil
	}
	bootVerifyAnsible = func(context.Context) error {
		order = append(order, "ansible")
		return nil
	}
	bootEnsureSSH = func(context.Context) (string, error) {
		order = append(order, "ssh")
		return "ready", nil
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || strings.Join(order, ",") != "locale,ansible,ssh" || data.Locale != "repaired" || !data.Changed ||
		!strings.Contains(progress.String(), "[ OK ] locale  = ready") {
		t.Fatalf("bootstrap order = %v, result=%+v", order, result)
	}
}

func TestBootstrapNativeLocaleFailureIsWarning(t *testing.T) {
	setupBootTest(t)
	config.OSType, config.OSCode, config.OSVendor, config.OSVersion = config.DistroDEB, "d12", "debian", "12"
	bootVerifyAnsible = func(context.Context) error { return nil }
	bootRepairLocale = func(context.Context) (bool, error) { return false, errors.New("locale-gen failed") }
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.Locale != "warning" || len(data.Warnings) != 1 ||
		!strings.Contains(data.Warnings[0], "locale-gen failed") {
		t.Fatalf("locale warning result = %+v %+v", result, data)
	}
}

func TestBootstrapNativeInitializesMissingDefaultPigsty(t *testing.T) {
	setupBootTest(t)
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
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
	if text := data.Text(); strings.Contains(text, "pigsty_init=") || !strings.Contains(text, "proceed with:") {
		t.Fatalf("auto-init text was not concise: %q", text)
	}
}

func TestBootstrapNativePigstyInitFailureIsWarning(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return nil }
	bootEnsurePigsty = func(context.Context, string) (pigstyInitOutcome, error) {
		return pigstyInitOutcome{Path: "/home/admin/pigsty", Status: pigstyInitFailed}, errors.New("download failed")
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || data.PigstyInit != pigstyInitFailed || len(data.Warnings) != 1 ||
		!strings.Contains(data.Warnings[0], "download failed") {
		t.Fatalf("auto-init failure result = %+v %+v", result, data)
	}
}

func TestBootstrapNativeMissingPigstyWithoutNetworkWarns(t *testing.T) {
	setupBootTest(t)
	bootVerifyAnsible = func(context.Context) error { return nil }
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
	want := []string{"pig sty conf -g", "pig inv edit", "pig sty deploy"}
	if len(result.NextActions) != len(want) {
		t.Fatalf("unavailable auto-init next actions = %+v", result.NextActions)
	}
	for i := range want {
		if result.NextActions[i].Command != want[i] {
			t.Fatalf("next action %d = %q, want %q", i, result.NextActions[i].Command, want[i])
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
			wantSOW := true
			if tc.osType == config.DistroEL && tc.version == "8" {
				wantSOW = false
				if containsPackage(packages, "python3") {
					t.Fatalf("EL8 requested the legacy bootstrap's implicit Python runtime: %v", packages)
				}
				for _, required := range []string{"python3.12-jmespath", "python3-cryptography"} {
					if !containsPackage(packages, required) {
						t.Fatalf("EL8 legacy bootstrap package %q missing: %v", required, packages)
					}
				}
				for _, unwanted := range []string{"python3.12-jinja2", "python3.12-requests", "python3.12-cryptography"} {
					if containsPackage(packages, unwanted) {
						t.Fatalf("EL8 requested package outside the legacy bootstrap set %q: %v", unwanted, packages)
					}
				}
			}
			if containsPackage(packages, "sow") != wantSOW {
				t.Fatalf("SOW package mismatch: want=%t packages=%v", wantSOW, packages)
			}
			if !containsPackage(packages, "ansible") ||
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

func TestBootstrapNativeRetriesLocaleRepairAfterControllerInstall(t *testing.T) {
	setupBootTest(t)
	config.OSType, config.OSCode, config.OSVendor, config.OSVersion = config.DistroDEB, "d12", "debian", "12"
	bootLookPath = func(string) (string, error) { return "/usr/bin/apt-get", nil }
	repairCalls := 0
	bootRepairLocale = func(context.Context) (bool, error) {
		repairCalls++
		if repairCalls == 1 {
			return false, errors.New("inspect /etc/locale.gen: no such file or directory")
		}
		return true, nil
	}
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if !result.Success || repairCalls != 2 || data.Locale != "repaired" || len(data.Warnings) != 0 {
		t.Fatalf("locale retry = %+v repairs=%d warnings=%v", data, repairCalls, data.Warnings)
	}
}

func TestBootstrapNativeOnlineFailureReportsRepoRootCause(t *testing.T) {
	setupBootTest(t)
	bootRepos = func([]string, repo.AddOptions) *output.Result {
		return output.Fail(output.CodeRepoAddFailed, "failed to install pigsty GPG key: boom")
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	if result.Success || !strings.Contains(result.Message, "failed to install pigsty GPG key: boom") {
		t.Fatalf("repository root cause was dropped: %+v", result)
	}
}

func TestBootstrapNativeOnlineRestoresReposAfterInstallFailure(t *testing.T) {
	setupBootTest(t)
	backup := &repo.BackupInfo{BackupDir: "/fixture/repo-backup", BackedUpFiles: []string{"/fixture/original.repo"}}
	bootRepos = func([]string, repo.AddOptions) *output.Result {
		return output.OK("added", &repo.RepoAddData{
			AddedRepos: []*repo.AddedRepoItem{
				{Module: "node", FilePath: "/etc/yum.repos.d/node.repo"},
				{Module: "pigsty", FilePath: "/etc/yum.repos.d/pigsty.repo"},
			},
			BackupInfo: backup,
		})
	}
	bootInstall = func(int, []string, install.PackageOptions) error { return errors.New("mirror unreachable") }
	restored := false
	bootRestoreRepos = func(got *repo.BackupInfo, added ...string) (*repo.RollbackInfo, error) {
		restored = true
		if got != backup || len(added) != 2 || added[0] != "/etc/yum.repos.d/node.repo" || added[1] != "/etc/yum.repos.d/pigsty.repo" {
			t.Fatalf("online restore args = %+v %v", got, added)
		}
		return &repo.RollbackInfo{Success: true, RestoredFiles: backup.BackedUpFiles}, nil
	}

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	if result.Success || !restored || data.RepositoryRollback != "restored" ||
		!strings.Contains(result.Message, "mirror unreachable") {
		t.Fatalf("online install rollback = %+v %+v restored=%t", result, data, restored)
	}
}

func TestBootstrapNativeNormalizesRegionCase(t *testing.T) {
	setupBootTest(t)
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	var gotOpts repo.AddOptions
	bootRepos = func(_ []string, opts repo.AddOptions) *output.Result {
		gotOpts = opts
		return successfulBootstrapRepoResult()
	}
	result := BootstrapNative(context.Background(), NativeBootstrapOptions{Region: " China "})
	if !result.Success || gotOpts.Region != "china" || !gotOpts.Mirror {
		t.Fatalf("region normalization = %+v opts=%+v", result, gotOpts)
	}
}

func TestBootstrapNativeWarnsWhenControllerHelpersAreMissing(t *testing.T) {
	setupBootTest(t)
	bootInstall = func(int, []string, install.PackageOptions) error { return nil }
	bootRepos = func([]string, repo.AddOptions) *output.Result { return successfulBootstrapRepoResult() }
	bootVerifyHelpers = func(context.Context) error { return errors.New("sshpass: executable file not found") }

	result := BootstrapNative(context.Background(), NativeBootstrapOptions{})
	data := result.Data.(*BootData)
	found := false
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "controller helpers") && strings.Contains(warning, "sshpass") {
			found = true
		}
	}
	if !result.Success || !found {
		t.Fatalf("controller helper warning missing: success=%t warnings=%v", result.Success, data.Warnings)
	}
}
