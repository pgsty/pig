package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/output"
	"strings"
	"testing"
)

func TestRmReposRejectsExplicitInvalidBeforeEffects(t *testing.T) {
	for _, args := range [][]string{{""}, {" "}, {","}, {"node,"}, {",node"}, {"node,,infra"}, {"node", "../evil"}, {"node", " "}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			manager, dir := setupAddRollbackTest(t)
			old := newRmRepoManager
			t.Cleanup(func() { newRmRepoManager = old })
			calls := 0
			newRmRepoManager = func(...string) (*Manager, error) { calls++; return manager, nil }
			t.Setenv("PIG_NO_SUDO", "1")
			manager.UpdateCmd = []string{"true"}
			writeRollbackFixture(t, filepath.Join(dir, "node.repo"), "original")
			result := RmRepos(args, true)
			if result.Success || result.Code != output.CodeRepoInvalidArgs {
				t.Errorf("invalid input result: %+v", result)
			}
			if calls != 0 {
				t.Error("invalid input initialized repository manager")
			}
			assertRollbackFixture(t, filepath.Join(dir, "node.repo"), "original")
		})
	}
}
func TestRmReposScopeAndCache(t *testing.T) {
	for _, args := range [][]string{nil, {"all"}, {"node, infra"}, {"custom"}, {"missing"}} {
		for _, update := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/update=%v", args, update), func(t *testing.T) {
				manager, dir := setupAddRollbackTest(t)
				old := newRmRepoManager
				t.Cleanup(func() { newRmRepoManager = old })
				newRmRepoManager = func(...string) (*Manager, error) { return manager, nil }
				t.Setenv("PIG_NO_SUDO", "1")
				marker := filepath.Join(t.TempDir(), "updated")
				manager.UpdateCmd = []string{"touch", marker}
				for _, name := range []string{"node", "pgsql", "infra", "custom"} {
					writeRollbackFixture(t, filepath.Join(dir, name+".repo"), name)
				}
				result := RmRepos(args, update)
				if !result.Success {
					t.Fatalf("result: %+v", result)
				}
				_, err := os.Stat(marker)
				if (err == nil) != update {
					t.Fatalf("cache update=%v err=%v", update, err)
				}
				data := result.Data.(*RepoRmData)
				if len(args) == 0 {
					if data.BackupInfo == nil || len(data.BackupInfo.BackedUpFiles) != 4 {
						t.Fatalf("backup=%+v", data.BackupInfo)
					}
					for _, name := range []string{"node", "pgsql", "infra", "custom"} {
						assertRollbackFixture(t, filepath.Join(manager.BackupDir, name+".repo"), name)
					}
				} else {
					if data.BackupInfo != nil {
						t.Fatal("explicit modules triggered full backup")
					}
					if args[0] == "all" {
						assertRollbackFixture(t, filepath.Join(dir, "custom.repo"), "custom")
					}
					if args[0] == "missing" && (!strings.Contains(result.Message, "absent") || !strings.Contains(data.Text(), "absent")) {
						t.Fatalf("idempotent removal claims a file was deleted: %s; %s", result.Message, data.Text())
					}
					for _, item := range data.RemovedRepos {
						if _, err := os.Stat(item.FilePath); !os.IsNotExist(err) {
							t.Errorf("not removed: %s", item.FilePath)
						}
					}
				}
			})
		}
	}
}

func TestRmReposEmptyBackupSummary(t *testing.T) {
	manager, _ := setupAddRollbackTest(t)
	old := newRmRepoManager
	t.Cleanup(func() { newRmRepoManager = old })
	newRmRepoManager = func(...string) (*Manager, error) { return manager, nil }
	result := RmRepos(nil, false)
	if !result.Success {
		t.Fatalf("empty backup failed: %+v", result)
	}
	if text := result.Data.(*RepoRmData).Text(); !strings.Contains(text, "No repository files to back up") || strings.Contains(text, "module(s)") {
		t.Fatalf("empty backup described module removal: %s", text)
	}
}

func TestRmReposBackupSurvivesCacheFailure(t *testing.T) {
	manager, dir := setupAddRollbackTest(t)
	old := newRmRepoManager
	t.Cleanup(func() { newRmRepoManager = old })
	newRmRepoManager = func(...string) (*Manager, error) { return manager, nil }
	t.Setenv("PIG_NO_SUDO", "1")
	manager.UpdateCmd = []string{"sh", "-c", "exit 17"}
	writeRollbackFixture(t, filepath.Join(dir, "custom.repo"), "original")
	result := RmRepos(nil, true)
	if result.Success || result.Code != output.CodeRepoCacheUpdateFailed {
		t.Fatalf("cache failure reported success: %+v", result)
	}
	data := result.Data.(*RepoRmData)
	if data.BackupInfo == nil || data.UpdateResult == nil || data.UpdateResult.Success || data.RollbackInfo != nil {
		t.Fatalf("backup/update contract changed: %+v", data)
	}
	assertRollbackFixture(t, filepath.Join(manager.BackupDir, "custom.repo"), "original")
	if _, err := os.Stat(filepath.Join(dir, "custom.repo")); !os.IsNotExist(err) {
		t.Fatalf("completed backup was undone after cache failure: %v", err)
	}
}
