package repo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pig/internal/output"
)

func setupAddRollbackTest(t *testing.T) (*Manager, string) {
	t.Helper()
	withELConfig(t, 9, "9", "9.6")
	manager, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	repoDir := t.TempDir()
	manager.RepoDir = repoDir
	manager.BackupDir = filepath.Join(repoDir, "backup")
	manager.UpdateCmd = []string{"cache-update"}

	origManager, origRun, origSummary, origAdd := newAddRepoManager, runAddRepoCommand, runAddRepoCommandSummary, addRepoModule
	t.Cleanup(func() {
		newAddRepoManager, runAddRepoCommand, runAddRepoCommandSummary, addRepoModule = origManager, origRun, origSummary, origAdd
	})
	newAddRepoManager = func(...string) (*Manager, error) { return manager, nil }
	runAddRepoCommand = runRepoTestCommand
	addRepoModule = func(manager *Manager, module string) error { return manager.AddModule(module) }
	return manager, repoDir
}

func TestAddReposQuietUsesSummaryRunnerForCacheUpdate(t *testing.T) {
	setupAddRollbackTest(t)
	runAddRepoCommand = func([]string) error {
		t.Fatal("quiet cache update used streaming command runner")
		return nil
	}
	var call []string
	runAddRepoCommandSummary = func(args []string) error {
		call = append([]string(nil), args...)
		return nil
	}

	result := AddReposWithOptions([]string{"node"}, AddOptions{Update: true, Quiet: true})
	if !result.Success || len(call) != 1 || call[0] != "cache-update" {
		t.Fatalf("quiet cache update result = %+v call=%v", result, call)
	}
}

func TestAddRepoModulesDoesNotRetryFailedDuplicate(t *testing.T) {
	manager, _ := setupAddRollbackTest(t)
	calls := 0
	addRepoModule = func(*Manager, string) error {
		calls++
		return errors.New("write failed")
	}
	data := &RepoAddData{}
	addRepoModules(manager, []string{"node", "node"}, data, false)
	if calls != 1 {
		t.Fatalf("failed duplicate module executed %d times, want 1", calls)
	}
	if len(data.Failed) != 1 || data.Failed[0].Module != "node" {
		t.Fatalf("failed modules = %+v, want one node failure", data.Failed)
	}
}

func TestNormalizeModulesDeduplicatesNonAdjacentInputs(t *testing.T) {
	manager, _ := setupAddRollbackTest(t)
	got := manager.normalizeModules("node", "pigsty", "node")
	want := []string{"node", "pigsty"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeModules() = %v, want %v", got, want)
	}
}

func runRepoTestCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("empty command")
	}
	return exec.Command(args[0], args[1:]...).Run()
}

func writeRollbackFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertRollbackFixture(t *testing.T, path, content string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored repository %s: %v", path, err)
	}
	if string(got) != content {
		t.Fatalf("restored repository %s = %q, want %q", path, got, content)
	}
}

func TestReplaceReposRollsBackAfterCacheFailure(t *testing.T) {
	_, repoDir := setupAddRollbackTest(t)
	legacyNode := "legacy node repository\n"
	legacyCustom := "legacy custom repository\n"
	writeRollbackFixture(t, filepath.Join(repoDir, "node.repo"), legacyNode)
	writeRollbackFixture(t, filepath.Join(repoDir, "custom.repo"), legacyCustom)

	runAddRepoCommand = func(args []string) error {
		if len(args) > 0 && args[0] == "cache-update" {
			return errors.New("repository is unreachable")
		}
		return runRepoTestCommand(args)
	}
	result := AddReposWithOptions([]string{"node", "pigsty"}, AddOptions{Remove: true, Update: true})
	if result.Success || result.Code != output.CodeRepoCacheUpdateFailed {
		t.Fatalf("cache failure result = %+v", result)
	}
	data := result.Data.(*RepoAddData)
	if data.RollbackInfo == nil || !data.RollbackInfo.Success {
		t.Fatalf("cache failure rollback = %+v", data.RollbackInfo)
	}
	if !strings.Contains(result.Detail, "previous repository definitions restored") {
		t.Fatalf("cache failure detail = %q", result.Detail)
	}
	assertRollbackFixture(t, filepath.Join(repoDir, "node.repo"), legacyNode)
	assertRollbackFixture(t, filepath.Join(repoDir, "custom.repo"), legacyCustom)
	if _, err := os.Stat(filepath.Join(repoDir, "pigsty.repo")); !os.IsNotExist(err) {
		t.Fatalf("failed replacement repository remains after rollback: %v", err)
	}
}

func TestReplaceReposResolvesRouteBeforeMovingDefinitions(t *testing.T) {
	manager, repoDir := setupAddRollbackTest(t)
	writeRollbackFixture(t, filepath.Join(repoDir, "legacy.repo"), "legacy\n")
	runAddRepoCommand = func(args []string) error {
		if len(args) > 0 && args[0] == "mv" && manager.Region != "europe" {
			t.Fatalf("repository definition moved before route resolution: region=%q args=%v", manager.Region, args)
		}
		return runRepoTestCommand(args)
	}

	result := AddReposWithOptions([]string{"node"}, AddOptions{Region: "europe", Remove: true})
	if !result.Success || manager.Region != "europe" {
		t.Fatalf("route-first replacement result = %+v region=%q", result, manager.Region)
	}
}

func TestReplaceReposRollsBackAfterModuleFailure(t *testing.T) {
	manager, repoDir := setupAddRollbackTest(t)
	legacy := "legacy repository\n"
	writeRollbackFixture(t, filepath.Join(repoDir, "legacy.repo"), legacy)
	updateCalls := 0
	runAddRepoCommand = func(args []string) error {
		if len(args) > 0 && args[0] == "cache-update" {
			updateCalls++
			return nil
		}
		return runRepoTestCommand(args)
	}
	addRepoModule = func(got *Manager, module string) error {
		if module == "pigsty" {
			return errors.New("write failed")
		}
		return got.AddModule(module)
	}

	result := AddReposWithOptions([]string{"node", "pigsty"}, AddOptions{Remove: true, Update: true})
	if result.Success || result.Code != output.CodeRepoAddFailed {
		t.Fatalf("module failure result = %+v", result)
	}
	data := result.Data.(*RepoAddData)
	if data.RollbackInfo == nil || !data.RollbackInfo.Success || updateCalls != 0 {
		t.Fatalf("module failure rollback = %+v update_calls=%d", data.RollbackInfo, updateCalls)
	}
	assertRollbackFixture(t, filepath.Join(repoDir, "legacy.repo"), legacy)
	for _, name := range []string{"node.repo", "pigsty.repo"} {
		if _, err := os.Stat(filepath.Join(repoDir, name)); !os.IsNotExist(err) {
			t.Fatalf("failed replacement %s remains after rollback: %v", name, err)
		}
	}
	_ = manager
}

func TestReplaceReposRestoresPartialBackupFailure(t *testing.T) {
	_, repoDir := setupAddRollbackTest(t)
	first := filepath.Join(repoDir, "a.repo")
	second := filepath.Join(repoDir, "b.repo")
	writeRollbackFixture(t, first, "a\n")
	writeRollbackFixture(t, second, "b\n")
	runAddRepoCommand = func(args []string) error {
		if len(args) >= 3 && args[0] == "mv" && args[2] == second {
			return errors.New("move failed")
		}
		return runRepoTestCommand(args)
	}

	result := AddReposWithOptions([]string{"node"}, AddOptions{Remove: true})
	if result.Success || result.Code != output.CodeRepoBackupFailed {
		t.Fatalf("partial backup result = %+v", result)
	}
	data := result.Data.(*RepoAddData)
	if data.RollbackInfo == nil || !data.RollbackInfo.Success {
		t.Fatalf("partial backup rollback = %+v", data.RollbackInfo)
	}
	assertRollbackFixture(t, first, "a\n")
	assertRollbackFixture(t, second, "b\n")
}

func TestReplaceReposReportsIncompleteRollback(t *testing.T) {
	_, repoDir := setupAddRollbackTest(t)
	writeRollbackFixture(t, filepath.Join(repoDir, "node.repo"), "legacy node\n")
	runAddRepoCommand = func(args []string) error {
		if len(args) > 0 && args[0] == "cache-update" {
			return errors.New("repository is unreachable")
		}
		if len(args) >= 3 && args[0] == "rm" && strings.HasSuffix(args[2], "pigsty.repo") {
			return errors.New("remove denied")
		}
		return runRepoTestCommand(args)
	}

	result := AddReposWithOptions([]string{"node", "pigsty"}, AddOptions{Remove: true, Update: true})
	data := result.Data.(*RepoAddData)
	if result.Success || data.RollbackInfo == nil || data.RollbackInfo.Success {
		t.Fatalf("incomplete rollback result = %+v data=%+v", result, data)
	}
	if !strings.Contains(data.RollbackInfo.Error, "remove denied") ||
		!strings.Contains(result.Detail, "repository rollback failed") {
		t.Fatalf("incomplete rollback diagnostics = %+v detail=%q", data.RollbackInfo, result.Detail)
	}
}

func TestRemoveAllReposRestoresPartialBackupFailure(t *testing.T) {
	manager, repoDir := setupAddRollbackTest(t)
	origRmManager := newRmRepoManager
	t.Cleanup(func() { newRmRepoManager = origRmManager })
	newRmRepoManager = func(...string) (*Manager, error) { return manager, nil }

	first := filepath.Join(repoDir, "a.repo")
	second := filepath.Join(repoDir, "b.repo")
	writeRollbackFixture(t, first, "a\n")
	writeRollbackFixture(t, second, "b\n")
	runAddRepoCommand = func(args []string) error {
		if len(args) >= 3 && args[0] == "mv" && args[2] == second {
			return errors.New("move failed")
		}
		return runRepoTestCommand(args)
	}

	result := RmRepos(nil, false)
	if result.Success || result.Code != output.CodeRepoBackupFailed {
		t.Fatalf("remove-all partial backup result = %+v", result)
	}
	data := result.Data.(*RepoRmData)
	if data.RollbackInfo == nil || !data.RollbackInfo.Success {
		t.Fatalf("remove-all partial backup rollback = %+v", data.RollbackInfo)
	}
	assertRollbackFixture(t, first, "a\n")
	assertRollbackFixture(t, second, "b\n")
}
