package repo

import (
	"strings"
	"testing"

	"pig/internal/output"
)

func TestBuildAddReposResultUpdateFailureReturnsFailure(t *testing.T) {
	data := &RepoAddData{
		AddedRepos: []*AddedRepoItem{
			{Module: "pgdg", FilePath: "/etc/apt/sources.list.d/pgdg.list"},
		},
		UpdateResult: &UpdateCacheResult{
			Command: "apt-get update",
			Success: false,
			Error:   "command failed: exit status 100",
		},
	}

	result := buildAddReposResult(data, []string{"pgdg"})
	if result.Success {
		t.Fatal("expected add result to fail when cache update fails")
	}
	if result.Code != output.CodeRepoCacheUpdateFailed {
		t.Fatalf("expected code %d, got %d", output.CodeRepoCacheUpdateFailed, result.Code)
	}
	if !strings.Contains(result.Message, "cache update failed") {
		t.Fatalf("expected cache update failure message, got %q", result.Message)
	}
}

func TestBuildAddReposResultSuccessKeepsOK(t *testing.T) {
	data := &RepoAddData{
		AddedRepos: []*AddedRepoItem{
			{Module: "pgdg", FilePath: "/etc/apt/sources.list.d/pgdg.list", Repos: []string{"pgdg"}},
		},
		UpdateResult: &UpdateCacheResult{
			Command: "apt-get update",
			Success: true,
		},
	}

	result := buildAddReposResult(data, []string{"pgdg"})
	if !result.Success {
		t.Fatal("expected add result success when add and cache update succeed")
	}
}

func TestBuildAddReposResultPartialFailureIsNonzero(t *testing.T) {
	data := &RepoAddData{
		AddedRepos: []*AddedRepoItem{{Module: "pgdg", Repos: []string{"pgdg"}}},
		Failed:     []*FailedRepoItem{{Module: "missing", Error: "not found"}},
	}
	result := buildAddReposResult(data, []string{"pgdg", "missing"})
	if result.Success || result.Code != output.CodeRepoAddFailed || !strings.Contains(result.Message, "1 requested") {
		t.Fatalf("partial add result = %+v", result)
	}
}

func TestBuildRmReposResultUpdateFailureReturnsFailure(t *testing.T) {
	data := &RepoRmData{
		RemovedRepos: []*RemovedRepoItem{
			{Module: "pgdg", FilePath: "/etc/apt/sources.list.d/pgdg.list", Success: true},
		},
		UpdateResult: &UpdateCacheResult{
			Command: "apt-get update",
			Success: false,
			Error:   "command failed: exit status 100",
		},
	}

	result := buildRmReposResult(data, 1)
	if result.Success {
		t.Fatal("expected rm result to fail when cache update fails")
	}
	if result.Code != output.CodeRepoCacheUpdateFailed {
		t.Fatalf("expected code %d, got %d", output.CodeRepoCacheUpdateFailed, result.Code)
	}
	if !strings.Contains(result.Message, "cache update failed") {
		t.Fatalf("expected cache update failure message, got %q", result.Message)
	}
}

func TestBuildRmReposResultRemoveFailureTakesPriority(t *testing.T) {
	data := &RepoRmData{
		RemovedRepos: []*RemovedRepoItem{
			{Module: "pgdg", FilePath: "/etc/apt/sources.list.d/pgdg.list", Success: false, Error: "rm failed"},
		},
		UpdateResult: &UpdateCacheResult{
			Command: "apt-get update",
			Success: false,
			Error:   "command failed: exit status 100",
		},
	}

	result := buildRmReposResult(data, 1)
	if result.Success {
		t.Fatal("expected rm result to fail when nothing is removed")
	}
	if result.Code != output.CodeRepoRemoveFailed {
		t.Fatalf("expected code %d, got %d", output.CodeRepoRemoveFailed, result.Code)
	}
}

func TestBuildRmReposResultPartialFailureIsNonzero(t *testing.T) {
	data := &RepoRmData{RemovedRepos: []*RemovedRepoItem{
		{Module: "pgdg", Success: true},
		{Module: "missing", Success: false, Error: "rm failed"},
	}}
	result := buildRmReposResult(data, 2)
	if result.Success || result.Code != output.CodeRepoRemoveFailed || !strings.Contains(result.Message, "1 requested") {
		t.Fatalf("partial remove result = %+v", result)
	}
}
