/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>
*/
package repo

import (
	"encoding/json"
	"pig/internal/output"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOSEnvironmentSerialization(t *testing.T) {
	env := &OSEnvironment{
		Code:  "el9",
		Arch:  "x86_64",
		Type:  "rpm",
		Major: 9,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonEnv OSEnvironment
	if err := json.Unmarshal(jsonData, &jsonEnv); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonEnv.Code != "el9" || jsonEnv.Arch != "x86_64" || jsonEnv.Type != "rpm" || jsonEnv.Major != 9 {
		t.Errorf("JSON roundtrip failed: got %+v", jsonEnv)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlEnv OSEnvironment
	if err := yaml.Unmarshal(yamlData, &yamlEnv); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlEnv.Code != "el9" || yamlEnv.Arch != "x86_64" || yamlEnv.Type != "rpm" || yamlEnv.Major != 9 {
		t.Errorf("YAML roundtrip failed: got %+v", yamlEnv)
	}
}

func TestRepoSummarySerialization(t *testing.T) {
	summary := &RepoSummary{
		Name:        "pigsty-pgsql",
		Description: "Pigsty PGSQL",
		Module:      "pgsql",
		Releases:    []int{7, 8, 9, 10},
		Arch:        []string{"x86_64", "aarch64"},
		BaseURL:     "https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch",
		Available:   true,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonSummary RepoSummary
	if err := json.Unmarshal(jsonData, &jsonSummary); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonSummary.Name != "pigsty-pgsql" || !jsonSummary.Available {
		t.Errorf("JSON roundtrip failed: got %+v", jsonSummary)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(summary)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlSummary RepoSummary
	if err := yaml.Unmarshal(yamlData, &yamlSummary); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlSummary.Name != "pigsty-pgsql" || len(yamlSummary.Releases) != 4 {
		t.Errorf("YAML roundtrip failed: got %+v", yamlSummary)
	}
}

func TestRepoDetailSerialization(t *testing.T) {
	detail := &RepoDetail{
		Name:        "pigsty-pgsql",
		Description: "Pigsty PGSQL",
		Module:      "pgsql",
		Releases:    []int{7, 8, 9, 10},
		Arch:        []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch",
			"china":   "https://repo.pigsty.cc/yum/pgsql/el$releasever.$basearch",
		},
		Meta: map[string]string{
			"enabled":  "1",
			"gpgcheck": "0",
		},
		Minor:     false,
		Available: true,
		Content:   "[pigsty-pgsql]\nname=pigsty-pgsql\n",
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonDetail RepoDetail
	if err := json.Unmarshal(jsonData, &jsonDetail); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonDetail.Name != "pigsty-pgsql" || len(jsonDetail.BaseURL) != 2 {
		t.Errorf("JSON roundtrip failed: got %+v", jsonDetail)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(detail)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlDetail RepoDetail
	if err := yaml.Unmarshal(yamlData, &yamlDetail); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlDetail.Meta["enabled"] != "1" || yamlDetail.Content == "" {
		t.Errorf("YAML roundtrip failed: got %+v", yamlDetail)
	}
}

func TestRepoListDataSerialization(t *testing.T) {
	data := &RepoListData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		ShowAll:   false,
		RepoCount: 2,
		Repos: []*RepoSummary{
			{
				Name:        "pigsty-infra",
				Description: "Pigsty INFRA",
				Module:      "infra",
				Available:   true,
			},
			{
				Name:        "pigsty-pgsql",
				Description: "Pigsty PGSQL",
				Module:      "pgsql",
				Available:   true,
			},
		},
		Modules: map[string][]string{
			"pigsty": {"pigsty-infra", "pigsty-pgsql"},
			"all":    {"pigsty-infra", "pigsty-pgsql", "pgdg"},
		},
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonListData RepoListData
	if err := json.Unmarshal(jsonData, &jsonListData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonListData.RepoCount != 2 || len(jsonListData.Repos) != 2 {
		t.Errorf("JSON roundtrip failed: got %+v", jsonListData)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlListData RepoListData
	if err := yaml.Unmarshal(yamlData, &yamlListData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if len(yamlListData.Modules) != 2 || yamlListData.OSEnv.Code != "el9" {
		t.Errorf("YAML roundtrip failed: got %+v", yamlListData)
	}
}

func TestRepoInfoDataSerialization(t *testing.T) {
	data := &RepoInfoData{
		Requested: []string{"pigsty", "pgdg"},
		Repos: []*RepoDetail{
			{
				Name:        "pigsty-pgsql",
				Description: "Pigsty PGSQL",
				Module:      "pgsql",
				Available:   true,
			},
		},
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonInfoData RepoInfoData
	if err := json.Unmarshal(jsonData, &jsonInfoData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(jsonInfoData.Requested) != 2 || len(jsonInfoData.Repos) != 1 {
		t.Errorf("JSON roundtrip failed: got %+v", jsonInfoData)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlInfoData RepoInfoData
	if err := yaml.Unmarshal(yamlData, &yamlInfoData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlInfoData.Repos[0].Name != "pigsty-pgsql" {
		t.Errorf("YAML roundtrip failed: got %+v", yamlInfoData)
	}
}

func TestRepoStatusDataSerialization(t *testing.T) {
	data := &RepoStatusData{
		OSType:      "rpm",
		RepoDir:     "/etc/yum.repos.d",
		RepoFiles:   []string{"pigsty.repo", "pgdg.repo"},
		ActiveRepos: []string{"pigsty-pgsql", "pgdg17"},
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonStatusData RepoStatusData
	if err := json.Unmarshal(jsonData, &jsonStatusData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonStatusData.OSType != "rpm" || len(jsonStatusData.RepoFiles) != 2 {
		t.Errorf("JSON roundtrip failed: got %+v", jsonStatusData)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlStatusData RepoStatusData
	if err := yaml.Unmarshal(yamlData, &yamlStatusData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlStatusData.RepoDir != "/etc/yum.repos.d" || len(yamlStatusData.ActiveRepos) != 2 {
		t.Errorf("YAML roundtrip failed: got %+v", yamlStatusData)
	}
}

func TestRepositoryToSummary(t *testing.T) {
	// Test nil receiver
	var nilRepo *Repository
	if nilRepo.ToSummary() != nil {
		t.Error("ToSummary on nil should return nil")
	}

	repo := &Repository{
		Name:        "pigsty-pgsql",
		Description: "Pigsty PGSQL",
		Module:      "pgsql",
		Releases:    []int{7, 8, 9, 10},
		Arch:        []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch",
		},
	}

	summary := repo.ToSummary()
	if summary == nil {
		t.Fatal("ToSummary should not return nil for valid repo")
	}

	if summary.Name != "pigsty-pgsql" {
		t.Errorf("Expected name 'pigsty-pgsql', got '%s'", summary.Name)
	}
	if summary.Module != "pgsql" {
		t.Errorf("Expected module 'pgsql', got '%s'", summary.Module)
	}
	if summary.BaseURL != "https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch" {
		t.Errorf("Unexpected baseurl: %s", summary.BaseURL)
	}
}

func TestRepositoryToDetail(t *testing.T) {
	// Test nil receiver
	var nilRepo *Repository
	if nilRepo.ToDetail() != nil {
		t.Error("ToDetail on nil should return nil")
	}

	repo := &Repository{
		Name:        "pigsty-pgsql",
		Description: "Pigsty PGSQL",
		Module:      "pgsql",
		Releases:    []int{7, 8, 9, 10},
		Arch:        []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch",
			"china":   "https://repo.pigsty.cc/yum/pgsql/el$releasever.$basearch",
		},
		Meta: map[string]string{
			"enabled": "1",
		},
		Minor:  true,
		Distro: "rpm",
	}

	detail := repo.ToDetail()
	if detail == nil {
		t.Fatal("ToDetail should not return nil for valid repo")
	}

	if detail.Name != "pigsty-pgsql" {
		t.Errorf("Expected name 'pigsty-pgsql', got '%s'", detail.Name)
	}
	if len(detail.BaseURL) != 2 {
		t.Errorf("Expected 2 baseurls, got %d", len(detail.BaseURL))
	}
	if !detail.Minor {
		t.Error("Expected Minor to be true")
	}
}

func TestListReposReturnsResult(t *testing.T) {
	// Test with showAll = false
	result := ListRepos(false)
	if result == nil {
		t.Fatal("ListRepos should not return nil")
	}

	// Result should be successful (manager should initialize even on non-Linux)
	if !result.Success {
		t.Logf("ListRepos returned failure (expected on non-Linux): %s", result.Message)
	}

	// Test with showAll = true
	resultAll := ListRepos(true)
	if resultAll == nil {
		t.Fatal("ListRepos(true) should not return nil")
	}
}

func TestGetRepoInfoEmptyArgs(t *testing.T) {
	result := GetRepoInfo([]string{})
	if result == nil {
		t.Fatal("GetRepoInfo should not return nil")
	}

	if result.Success {
		t.Error("GetRepoInfo with empty args should fail")
	}

	if result.Code != output.CodeRepoInvalidArgs {
		t.Errorf("Expected code %d (CodeRepoInvalidArgs), got %d", output.CodeRepoInvalidArgs, result.Code)
	}
}

func TestGetRepoInfoValidArgs(t *testing.T) {
	result := GetRepoInfo([]string{"pigsty"})
	if result == nil {
		t.Fatal("GetRepoInfo should not return nil")
	}

	// Should succeed as pigsty module exists
	if !result.Success {
		t.Logf("GetRepoInfo returned: %s (may be expected depending on manager init)", result.Message)
	}
}

func TestGetRepoStatusUnsupportedOS(t *testing.T) {
	result := GetRepoStatus()
	if result == nil {
		t.Fatal("GetRepoStatus should not return nil")
	}

	// On macOS, this should fail with unsupported OS error
	// On Linux EL/DEB, it might succeed or fail depending on file access
	if !result.Success {
		// Expected on non-Linux
		if result.Code == output.CodeRepoUnsupportedOS {
			t.Log("GetRepoStatus correctly returns unsupported OS error on non-Linux")
		} else {
			t.Logf("GetRepoStatus returned unexpected error code: %d - %s", result.Code, result.Message)
		}
	}
}

func TestIsNumericPriority(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"500", true},
		{"100", true},
		{"990", true},
		{"1000", true},
		{"1", true},
		{"0", true},
		{"999999", true},
		{"", false},
		{"abc", false},
		{"500abc", false},
		{"abc500", false},
		{"-100", false},
		{"5.0", false},
		{" 500", false},
		{"500 ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isNumericPriority(tt.input)
			if result != tt.expected {
				t.Errorf("isNumericPriority(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestJSONSnakeCaseTags(t *testing.T) {
	env := &OSEnvironment{
		Code:  "el9",
		Arch:  "x86_64",
		Type:  "rpm",
		Major: 9,
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)

	// Check for snake_case fields (not camelCase)
	if !strings.Contains(jsonStr, `"code"`) {
		t.Error("JSON should contain 'code' field")
	}
	if !strings.Contains(jsonStr, `"arch"`) {
		t.Error("JSON should contain 'arch' field")
	}
	if !strings.Contains(jsonStr, `"type"`) {
		t.Error("JSON should contain 'type' field")
	}
	if !strings.Contains(jsonStr, `"major"`) {
		t.Error("JSON should contain 'major' field")
	}

	// Check RepoListData
	data := &RepoListData{
		OSEnv:     env,
		ShowAll:   true,
		RepoCount: 5,
	}

	jsonData2, _ := json.Marshal(data)
	jsonStr2 := string(jsonData2)

	if !strings.Contains(jsonStr2, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field (snake_case)")
	}
	if !strings.Contains(jsonStr2, `"show_all"`) {
		t.Error("JSON should contain 'show_all' field (snake_case)")
	}
	if !strings.Contains(jsonStr2, `"repo_count"`) {
		t.Error("JSON should contain 'repo_count' field (snake_case)")
	}
}

/********************
 * Tests for repo add/set DTOs (Story 4.2)
 ********************/

func TestRepoAddDataSerialization(t *testing.T) {
	data := &RepoAddData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		Region:           "default",
		RequestedModules: []string{"all"},
		ExpandedModules:  []string{"infra", "node", "pgsql"},
		AddedRepos: []*AddedRepoItem{
			{
				Module:   "node",
				FilePath: "/etc/yum.repos.d/node.repo",
				Repos:    []string{"baseos", "appstream", "epel"},
			},
			{
				Module:   "pgsql",
				FilePath: "/etc/yum.repos.d/pgsql.repo",
				Repos:    []string{"pigsty-pgsql", "pgdg17"},
			},
		},
		Failed: nil,
		BackupInfo: &BackupInfo{
			BackupDir:     "/etc/yum.repos.d/backup",
			BackedUpFiles: []string{"/etc/yum.repos.d/old.repo"},
		},
		UpdateResult: &UpdateCacheResult{
			Command: "dnf makecache",
			Success: true,
		},
		DurationMs: 1234,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonAddData RepoAddData
	if err := json.Unmarshal(jsonData, &jsonAddData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonAddData.Region != "default" {
		t.Errorf("Expected region 'default', got '%s'", jsonAddData.Region)
	}
	if len(jsonAddData.ExpandedModules) != 3 {
		t.Errorf("Expected 3 expanded modules, got %d", len(jsonAddData.ExpandedModules))
	}
	if len(jsonAddData.AddedRepos) != 2 {
		t.Errorf("Expected 2 added repos, got %d", len(jsonAddData.AddedRepos))
	}
	if jsonAddData.DurationMs != 1234 {
		t.Errorf("Expected duration_ms 1234, got %d", jsonAddData.DurationMs)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlAddData RepoAddData
	if err := yaml.Unmarshal(yamlData, &yamlAddData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlAddData.BackupInfo == nil {
		t.Error("Expected BackupInfo to be present")
	} else if yamlAddData.BackupInfo.BackupDir != "/etc/yum.repos.d/backup" {
		t.Errorf("Unexpected backup_dir: %s", yamlAddData.BackupInfo.BackupDir)
	}
	if yamlAddData.UpdateResult == nil {
		t.Error("Expected UpdateResult to be present")
	} else if !yamlAddData.UpdateResult.Success {
		t.Error("Expected UpdateResult.Success to be true")
	}
}

func TestAddedRepoItemSerialization(t *testing.T) {
	item := &AddedRepoItem{
		Module:   "pgsql",
		FilePath: "/etc/yum.repos.d/pgsql.repo",
		Repos:    []string{"pigsty-pgsql", "pgdg17", "pgdg-common"},
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"module"`) {
		t.Error("JSON should contain 'module' field")
	}
	if !strings.Contains(jsonStr, `"file_path"`) {
		t.Error("JSON should contain 'file_path' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"repos"`) {
		t.Error("JSON should contain 'repos' field")
	}

	var jsonItem AddedRepoItem
	if err := json.Unmarshal(jsonData, &jsonItem); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonItem.Module != "pgsql" {
		t.Errorf("Expected module 'pgsql', got '%s'", jsonItem.Module)
	}
	if len(jsonItem.Repos) != 3 {
		t.Errorf("Expected 3 repos, got %d", len(jsonItem.Repos))
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(item)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlItem AddedRepoItem
	if err := yaml.Unmarshal(yamlData, &yamlItem); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlItem.FilePath != "/etc/yum.repos.d/pgsql.repo" {
		t.Errorf("Unexpected file_path: %s", yamlItem.FilePath)
	}
}

func TestFailedRepoItemSerialization(t *testing.T) {
	item := &FailedRepoItem{
		Module: "nonexistent",
		Error:  "module not found: nonexistent",
		Code:   output.CodeRepoModuleNotFound,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"module"`) {
		t.Error("JSON should contain 'module' field")
	}
	if !strings.Contains(jsonStr, `"error"`) {
		t.Error("JSON should contain 'error' field")
	}
	if !strings.Contains(jsonStr, `"code"`) {
		t.Error("JSON should contain 'code' field")
	}

	var jsonItem FailedRepoItem
	if err := json.Unmarshal(jsonData, &jsonItem); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonItem.Module != "nonexistent" {
		t.Errorf("Expected module 'nonexistent', got '%s'", jsonItem.Module)
	}
	if jsonItem.Code != output.CodeRepoModuleNotFound {
		t.Errorf("Expected code %d, got %d", output.CodeRepoModuleNotFound, jsonItem.Code)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(item)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlItem FailedRepoItem
	if err := yaml.Unmarshal(yamlData, &yamlItem); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlItem.Error != "module not found: nonexistent" {
		t.Errorf("Unexpected error: %s", yamlItem.Error)
	}
}

func TestBackupInfoSerialization(t *testing.T) {
	info := &BackupInfo{
		BackupDir:     "/etc/yum.repos.d/backup",
		BackedUpFiles: []string{"/etc/yum.repos.d/epel.repo", "/etc/yum.repos.d/pgdg.repo"},
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"backup_dir"`) {
		t.Error("JSON should contain 'backup_dir' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"backed_up_files"`) {
		t.Error("JSON should contain 'backed_up_files' field (snake_case)")
	}

	var jsonInfo BackupInfo
	if err := json.Unmarshal(jsonData, &jsonInfo); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(jsonInfo.BackedUpFiles) != 2 {
		t.Errorf("Expected 2 backed up files, got %d", len(jsonInfo.BackedUpFiles))
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(info)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlInfo BackupInfo
	if err := yaml.Unmarshal(yamlData, &yamlInfo); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlInfo.BackupDir != "/etc/yum.repos.d/backup" {
		t.Errorf("Unexpected backup_dir: %s", yamlInfo.BackupDir)
	}
}

func TestBackupInfoIsEmpty(t *testing.T) {
	// Test nil receiver
	var nilInfo *BackupInfo
	if !nilInfo.IsEmpty() {
		t.Error("IsEmpty on nil should return true")
	}

	// Test empty BackedUpFiles
	emptyInfo := &BackupInfo{
		BackupDir:     "/backup",
		BackedUpFiles: []string{},
	}
	if !emptyInfo.IsEmpty() {
		t.Error("IsEmpty on empty BackedUpFiles should return true")
	}

	// Test non-empty BackedUpFiles
	nonEmptyInfo := &BackupInfo{
		BackupDir:     "/backup",
		BackedUpFiles: []string{"/file.repo"},
	}
	if nonEmptyInfo.IsEmpty() {
		t.Error("IsEmpty on non-empty BackedUpFiles should return false")
	}
}

func TestUpdateCacheResultSerialization(t *testing.T) {
	// Test successful result
	successResult := &UpdateCacheResult{
		Command: "dnf makecache",
		Success: true,
	}

	jsonData, err := json.Marshal(successResult)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"command"`) {
		t.Error("JSON should contain 'command' field")
	}
	if !strings.Contains(jsonStr, `"success"`) {
		t.Error("JSON should contain 'success' field")
	}
	// Error should not be present when empty (omitempty)
	if strings.Contains(jsonStr, `"error"`) {
		t.Error("JSON should not contain 'error' field when empty")
	}

	var jsonResult UpdateCacheResult
	if err := json.Unmarshal(jsonData, &jsonResult); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if !jsonResult.Success {
		t.Error("Expected Success to be true")
	}

	// Test failure result
	failResult := &UpdateCacheResult{
		Command: "apt update",
		Success: false,
		Error:   "failed to fetch packages",
	}

	jsonData2, _ := json.Marshal(failResult)
	jsonStr2 := string(jsonData2)
	if !strings.Contains(jsonStr2, `"error"`) {
		t.Error("JSON should contain 'error' field when present")
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(failResult)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlResult UpdateCacheResult
	if err := yaml.Unmarshal(yamlData, &yamlResult); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlResult.Success {
		t.Error("Expected Success to be false")
	}
	if yamlResult.Error != "failed to fetch packages" {
		t.Errorf("Unexpected error: %s", yamlResult.Error)
	}
}

/********************
 * Tests for repo rm/update DTOs (Story 4.3)
 ********************/

func TestRepoRmDataSerialization(t *testing.T) {
	data := &RepoRmData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		RequestedModules: []string{"pigsty", "pgdg"},
		RemovedRepos: []*RemovedRepoItem{
			{
				Module:   "pigsty",
				FilePath: "/etc/yum.repos.d/pigsty.repo",
				Success:  true,
			},
			{
				Module:   "pgdg",
				FilePath: "/etc/yum.repos.d/pgdg.repo",
				Success:  true,
			},
		},
		BackupInfo: nil,
		UpdateResult: &UpdateCacheResult{
			Command: "dnf makecache",
			Success: true,
		},
		DurationMs: 567,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var jsonRmData RepoRmData
	if err := json.Unmarshal(jsonData, &jsonRmData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(jsonRmData.RequestedModules) != 2 {
		t.Errorf("Expected 2 requested modules, got %d", len(jsonRmData.RequestedModules))
	}
	if len(jsonRmData.RemovedRepos) != 2 {
		t.Errorf("Expected 2 removed repos, got %d", len(jsonRmData.RemovedRepos))
	}
	if jsonRmData.DurationMs != 567 {
		t.Errorf("Expected duration_ms 567, got %d", jsonRmData.DurationMs)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlRmData RepoRmData
	if err := yaml.Unmarshal(yamlData, &yamlRmData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlRmData.UpdateResult == nil {
		t.Error("Expected UpdateResult to be present")
	} else if !yamlRmData.UpdateResult.Success {
		t.Error("Expected UpdateResult.Success to be true")
	}
}

func TestRepoRmDataWithBackup(t *testing.T) {
	// Test case when backup is performed (no modules specified)
	data := &RepoRmData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		RequestedModules: []string{},
		RemovedRepos:     []*RemovedRepoItem{},
		BackupInfo: &BackupInfo{
			BackupDir:     "/etc/yum.repos.d/backup",
			BackedUpFiles: []string{"/etc/yum.repos.d/epel.repo", "/etc/yum.repos.d/pgdg.repo"},
		},
		DurationMs: 234,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"backup_info"`) {
		t.Error("JSON should contain 'backup_info' field")
	}
	if !strings.Contains(jsonStr, `"backup_dir"`) {
		t.Error("JSON should contain 'backup_dir' field")
	}

	var jsonRmData RepoRmData
	if err := json.Unmarshal(jsonData, &jsonRmData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonRmData.BackupInfo == nil {
		t.Fatal("BackupInfo should not be nil")
	}
	if len(jsonRmData.BackupInfo.BackedUpFiles) != 2 {
		t.Errorf("Expected 2 backed up files, got %d", len(jsonRmData.BackupInfo.BackedUpFiles))
	}
}

func TestRemovedRepoItemSerialization(t *testing.T) {
	// Test successful removal
	item := &RemovedRepoItem{
		Module:   "pigsty",
		FilePath: "/etc/yum.repos.d/pigsty.repo",
		Success:  true,
	}

	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"module"`) {
		t.Error("JSON should contain 'module' field")
	}
	if !strings.Contains(jsonStr, `"file_path"`) {
		t.Error("JSON should contain 'file_path' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"success"`) {
		t.Error("JSON should contain 'success' field")
	}
	// Error should not be present when empty (omitempty)
	if strings.Contains(jsonStr, `"error"`) {
		t.Error("JSON should not contain 'error' field when empty")
	}

	var jsonItem RemovedRepoItem
	if err := json.Unmarshal(jsonData, &jsonItem); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonItem.Module != "pigsty" {
		t.Errorf("Expected module 'pigsty', got '%s'", jsonItem.Module)
	}
	if !jsonItem.Success {
		t.Error("Expected Success to be true")
	}

	// Test failed removal
	failedItem := &RemovedRepoItem{
		Module:   "nonexistent",
		FilePath: "/etc/yum.repos.d/nonexistent.repo",
		Success:  false,
		Error:    "failed to determine module path",
	}

	jsonData2, _ := json.Marshal(failedItem)
	jsonStr2 := string(jsonData2)
	if !strings.Contains(jsonStr2, `"error"`) {
		t.Error("JSON should contain 'error' field when present")
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(failedItem)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlItem RemovedRepoItem
	if err := yaml.Unmarshal(yamlData, &yamlItem); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlItem.Success {
		t.Error("Expected Success to be false")
	}
	if yamlItem.Error != "failed to determine module path" {
		t.Errorf("Unexpected error: %s", yamlItem.Error)
	}
}

func TestRemovedRepoItemText(t *testing.T) {
	// Test nil receiver
	var nilItem *RemovedRepoItem
	if nilItem.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test successful item
	successItem := &RemovedRepoItem{
		Module:   "pigsty",
		FilePath: "/etc/yum.repos.d/pigsty.repo",
		Success:  true,
	}
	if successItem.Text() != "/etc/yum.repos.d/pigsty.repo" {
		t.Errorf("Expected file path, got '%s'", successItem.Text())
	}

	// Test failed item
	failedItem := &RemovedRepoItem{
		Module:   "pigsty",
		FilePath: "/etc/yum.repos.d/pigsty.repo",
		Success:  false,
		Error:    "permission denied",
	}
	text := failedItem.Text()
	if !strings.Contains(text, "permission denied") {
		t.Errorf("Expected error in text, got '%s'", text)
	}
}

func TestRepoUpdateDataSerialization(t *testing.T) {
	// Test successful update
	data := &RepoUpdateData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		Command:    "dnf makecache",
		Success:    true,
		DurationMs: 5000,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field")
	}
	if !strings.Contains(jsonStr, `"command"`) {
		t.Error("JSON should contain 'command' field")
	}
	if !strings.Contains(jsonStr, `"success"`) {
		t.Error("JSON should contain 'success' field")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field")
	}
	// Error should not be present when empty (omitempty)
	if strings.Contains(jsonStr, `"error"`) {
		t.Error("JSON should not contain 'error' field when empty")
	}

	var jsonData2 RepoUpdateData
	if err := json.Unmarshal(jsonData, &jsonData2); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if !jsonData2.Success {
		t.Error("Expected Success to be true")
	}
	if jsonData2.Command != "dnf makecache" {
		t.Errorf("Expected command 'dnf makecache', got '%s'", jsonData2.Command)
	}

	// Test failed update
	failedData := &RepoUpdateData{
		OSEnv: &OSEnvironment{
			Code:  "u24",
			Arch:  "amd64",
			Type:  "deb",
			Major: 24,
		},
		Command:    "apt-get update",
		Success:    false,
		Error:      "network timeout",
		DurationMs: 30000,
	}

	jsonData3, _ := json.Marshal(failedData)
	jsonStr3 := string(jsonData3)
	if !strings.Contains(jsonStr3, `"error"`) {
		t.Error("JSON should contain 'error' field when present")
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(failedData)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlData2 RepoUpdateData
	if err := yaml.Unmarshal(yamlData, &yamlData2); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlData2.Success {
		t.Error("Expected Success to be false")
	}
	if yamlData2.Error != "network timeout" {
		t.Errorf("Unexpected error: %s", yamlData2.Error)
	}
}

func TestRepoUpdateDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoUpdateData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test successful update
	successData := &RepoUpdateData{
		Command: "dnf makecache",
		Success: true,
	}
	text := successData.Text()
	if !strings.Contains(text, "successfully") {
		t.Errorf("Expected success message, got '%s'", text)
	}
	if !strings.Contains(text, "dnf makecache") {
		t.Errorf("Expected command in text, got '%s'", text)
	}

	// Test failed update
	failedData := &RepoUpdateData{
		Command: "apt-get update",
		Success: false,
		Error:   "network timeout",
	}
	text2 := failedData.Text()
	if !strings.Contains(text2, "failed") {
		t.Errorf("Expected failure message, got '%s'", text2)
	}
	if !strings.Contains(text2, "network timeout") {
		t.Errorf("Expected error in text, got '%s'", text2)
	}
}

func TestRepoRmDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoRmData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test with backup info
	dataWithBackup := &RepoRmData{
		BackupInfo: &BackupInfo{
			BackupDir:     "/etc/yum.repos.d/backup",
			BackedUpFiles: []string{"/etc/yum.repos.d/epel.repo", "/etc/yum.repos.d/pgdg.repo"},
		},
	}
	text := dataWithBackup.Text()
	if !strings.Contains(text, "2 files") {
		t.Errorf("Expected '2 files' in text, got '%s'", text)
	}
	if !strings.Contains(text, "/etc/yum.repos.d/backup") {
		t.Errorf("Expected backup dir in text, got '%s'", text)
	}

	// Test with removed repos
	dataWithRemoved := &RepoRmData{
		RemovedRepos: []*RemovedRepoItem{
			{Module: "pigsty", FilePath: "/etc/yum.repos.d/pigsty.repo", Success: true},
			{Module: "pgdg", FilePath: "/etc/yum.repos.d/pgdg.repo", Success: true},
			{Module: "failed", FilePath: "/etc/yum.repos.d/failed.repo", Success: false},
		},
	}
	text2 := dataWithRemoved.Text()
	if !strings.Contains(text2, "2 module") {
		t.Errorf("Expected '2 module' in text, got '%s'", text2)
	}
}

func TestRepoRmDataOmitempty(t *testing.T) {
	// Test that optional fields are omitted when nil/empty
	data := &RepoRmData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		RequestedModules: []string{"pigsty"},
		RemovedRepos: []*RemovedRepoItem{
			{Module: "pigsty", FilePath: "/etc/yum.repos.d/pigsty.repo", Success: true},
		},
		BackupInfo:   nil, // Should be omitted
		UpdateResult: nil, // Should be omitted
		DurationMs:   100,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)

	// These fields should NOT appear when nil
	if strings.Contains(jsonStr, `"backup_info"`) {
		t.Error("JSON should not contain 'backup_info' field when nil")
	}
	if strings.Contains(jsonStr, `"update_result"`) {
		t.Error("JSON should not contain 'update_result' field when nil")
	}

	// These required fields SHOULD appear
	if !strings.Contains(jsonStr, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field")
	}
	if !strings.Contains(jsonStr, `"requested_modules"`) {
		t.Error("JSON should contain 'requested_modules' field")
	}
	if !strings.Contains(jsonStr, `"removed_repos"`) {
		t.Error("JSON should contain 'removed_repos' field")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field")
	}
}

func TestRmReposUnsupportedOS(t *testing.T) {
	// On macOS, this should return unsupported OS error
	result := RmRepos([]string{"pigsty"}, false)
	if result == nil {
		t.Fatal("RmRepos should not return nil")
	}

	// On non-Linux systems, expect unsupported OS error
	if !result.Success {
		if result.Code == output.CodeRepoUnsupportedOS {
			t.Log("RmRepos correctly returns unsupported OS error on non-Linux")
		} else if result.Code == output.CodeRepoManagerError {
			t.Log("RmRepos returned manager error (expected on some systems)")
		} else {
			t.Logf("RmRepos returned unexpected error code: %d - %s", result.Code, result.Message)
		}
	}
}

func TestUpdateCacheUnsupportedOS(t *testing.T) {
	// On macOS, this should return unsupported OS error
	result := UpdateCache()
	if result == nil {
		t.Fatal("UpdateCache should not return nil")
	}

	// On non-Linux systems, expect unsupported OS error
	if !result.Success {
		if result.Code == output.CodeRepoUnsupportedOS {
			t.Log("UpdateCache correctly returns unsupported OS error on non-Linux")
		} else if result.Code == output.CodeRepoManagerError {
			t.Log("UpdateCache returned manager error (expected on some systems)")
		} else {
			t.Logf("UpdateCache returned unexpected error code: %d - %s", result.Code, result.Message)
		}
	}
}

func TestRepoAddDataOmitempty(t *testing.T) {
	// Test that optional fields are omitted when nil/empty
	data := &RepoAddData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		Region:           "default",
		RequestedModules: []string{"pgsql"},
		ExpandedModules:  []string{"pgsql"},
		AddedRepos: []*AddedRepoItem{
			{Module: "pgsql", FilePath: "/etc/yum.repos.d/pgsql.repo", Repos: []string{"pigsty-pgsql"}},
		},
		Failed:       nil, // Should be omitted
		BackupInfo:   nil, // Should be omitted
		UpdateResult: nil, // Should be omitted
		DurationMs:   500,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)

	// These fields should NOT appear when nil
	if strings.Contains(jsonStr, `"failed"`) {
		t.Error("JSON should not contain 'failed' field when nil")
	}
	if strings.Contains(jsonStr, `"backup_info"`) {
		t.Error("JSON should not contain 'backup_info' field when nil")
	}
	if strings.Contains(jsonStr, `"update_result"`) {
		t.Error("JSON should not contain 'update_result' field when nil")
	}

	// These required fields SHOULD appear
	if !strings.Contains(jsonStr, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field")
	}
	if !strings.Contains(jsonStr, `"region"`) {
		t.Error("JSON should contain 'region' field")
	}
	if !strings.Contains(jsonStr, `"requested_modules"`) {
		t.Error("JSON should contain 'requested_modules' field")
	}
	if !strings.Contains(jsonStr, `"expanded_modules"`) {
		t.Error("JSON should contain 'expanded_modules' field")
	}
	if !strings.Contains(jsonStr, `"added_repos"`) {
		t.Error("JSON should contain 'added_repos' field")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field")
	}
}

/********************
 * Tests for repo create/boot/cache/reload DTOs (Story 4.4)
 ********************/

func TestRepoCreateDataSerialization(t *testing.T) {
	data := &RepoCreateData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		RepoDirs: []string{"/www/pigsty", "/www/mssql"},
		CreatedRepos: []*CreatedRepoItem{
			{
				Path:         "/www/pigsty",
				RepoType:     "yum",
				CompleteFile: "/www/pigsty/repo_complete",
			},
			{
				Path:         "/www/mssql",
				RepoType:     "yum",
				CompleteFile: "/www/mssql/repo_complete",
			},
		},
		FailedRepos: nil,
		DurationMs:  3456,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field")
	}
	if !strings.Contains(jsonStr, `"repo_dirs"`) {
		t.Error("JSON should contain 'repo_dirs' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"created_repos"`) {
		t.Error("JSON should contain 'created_repos' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field (snake_case)")
	}
	// failed_repos should not be present when nil (omitempty)
	if strings.Contains(jsonStr, `"failed_repos"`) {
		t.Error("JSON should not contain 'failed_repos' field when nil")
	}

	var jsonCreateData RepoCreateData
	if err := json.Unmarshal(jsonData, &jsonCreateData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(jsonCreateData.RepoDirs) != 2 {
		t.Errorf("Expected 2 repo_dirs, got %d", len(jsonCreateData.RepoDirs))
	}
	if len(jsonCreateData.CreatedRepos) != 2 {
		t.Errorf("Expected 2 created_repos, got %d", len(jsonCreateData.CreatedRepos))
	}
	if jsonCreateData.DurationMs != 3456 {
		t.Errorf("Expected duration_ms 3456, got %d", jsonCreateData.DurationMs)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlCreateData RepoCreateData
	if err := yaml.Unmarshal(yamlData, &yamlCreateData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlCreateData.CreatedRepos[0].Path != "/www/pigsty" {
		t.Errorf("Unexpected path: %s", yamlCreateData.CreatedRepos[0].Path)
	}
	if yamlCreateData.CreatedRepos[0].RepoType != "yum" {
		t.Errorf("Unexpected repo_type: %s", yamlCreateData.CreatedRepos[0].RepoType)
	}
}

func TestCreatedRepoItemSerialization(t *testing.T) {
	item := &CreatedRepoItem{
		Path:         "/www/pigsty",
		RepoType:     "yum",
		CompleteFile: "/www/pigsty/repo_complete",
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"path"`) {
		t.Error("JSON should contain 'path' field")
	}
	if !strings.Contains(jsonStr, `"repo_type"`) {
		t.Error("JSON should contain 'repo_type' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"complete_file"`) {
		t.Error("JSON should contain 'complete_file' field (snake_case)")
	}

	var jsonItem CreatedRepoItem
	if err := json.Unmarshal(jsonData, &jsonItem); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonItem.Path != "/www/pigsty" {
		t.Errorf("Expected path '/www/pigsty', got '%s'", jsonItem.Path)
	}
	if jsonItem.RepoType != "yum" {
		t.Errorf("Expected repo_type 'yum', got '%s'", jsonItem.RepoType)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(item)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlItem CreatedRepoItem
	if err := yaml.Unmarshal(yamlData, &yamlItem); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlItem.CompleteFile != "/www/pigsty/repo_complete" {
		t.Errorf("Unexpected complete_file: %s", yamlItem.CompleteFile)
	}
}

func TestCreatedRepoItemText(t *testing.T) {
	// Test nil receiver
	var nilItem *CreatedRepoItem
	if nilItem.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test normal item
	item := &CreatedRepoItem{
		Path:         "/www/pigsty",
		RepoType:     "yum",
		CompleteFile: "/www/pigsty/repo_complete",
	}
	if item.Text() != "/www/pigsty" {
		t.Errorf("Expected '/www/pigsty', got '%s'", item.Text())
	}
}

func TestFailedRepoItemTextForCreate(t *testing.T) {
	// Test nil receiver
	var nilItem *FailedRepoItem
	if nilItem.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test with module
	itemWithModule := &FailedRepoItem{
		Module: "/www/pigsty",
		Error:  "permission denied",
		Code:   output.CodeRepoCreateFailed,
	}
	text := itemWithModule.Text()
	if !strings.Contains(text, "/www/pigsty") {
		t.Errorf("Expected module in text, got '%s'", text)
	}
	if !strings.Contains(text, "permission denied") {
		t.Errorf("Expected error in text, got '%s'", text)
	}

	// Test without module
	itemWithoutModule := &FailedRepoItem{
		Error: "generic error",
		Code:  output.CodeRepoCreateFailed,
	}
	if itemWithoutModule.Text() != "generic error" {
		t.Errorf("Expected just error, got '%s'", itemWithoutModule.Text())
	}
}

func TestRepoCreateDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoCreateData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test successful creation
	data := &RepoCreateData{
		CreatedRepos: []*CreatedRepoItem{
			{Path: "/www/pigsty"},
			{Path: "/www/mssql"},
		},
	}
	text := data.Text()
	if !strings.Contains(text, "2 repos") {
		t.Errorf("Expected '2 repos' in text, got '%s'", text)
	}

	// Test with failures
	dataWithFail := &RepoCreateData{
		CreatedRepos: []*CreatedRepoItem{
			{Path: "/www/pigsty"},
		},
		FailedRepos: []*FailedRepoItem{
			{Module: "/www/mssql", Error: "error"},
		},
	}
	textFail := dataWithFail.Text()
	if !strings.Contains(textFail, "1 repos") {
		t.Errorf("Expected '1 repos' in text, got '%s'", textFail)
	}
	if !strings.Contains(textFail, "failed 1") {
		t.Errorf("Expected 'failed 1' in text, got '%s'", textFail)
	}
}

func TestRepoBootDataSerialization(t *testing.T) {
	data := &RepoBootData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		SourcePkg:      "/tmp/pkg.tgz",
		TargetDir:      "/www",
		ExtractedFiles: []string{"pigsty", "mssql"},
		LocalRepoAdded: true,
		LocalRepoPath:  "/etc/yum.repos.d/local.repo",
		DurationMs:     5678,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field")
	}
	if !strings.Contains(jsonStr, `"source_pkg"`) {
		t.Error("JSON should contain 'source_pkg' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"target_dir"`) {
		t.Error("JSON should contain 'target_dir' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"extracted_files"`) {
		t.Error("JSON should contain 'extracted_files' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"local_repo_added"`) {
		t.Error("JSON should contain 'local_repo_added' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"local_repo_path"`) {
		t.Error("JSON should contain 'local_repo_path' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field (snake_case)")
	}

	var jsonBootData RepoBootData
	if err := json.Unmarshal(jsonData, &jsonBootData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonBootData.SourcePkg != "/tmp/pkg.tgz" {
		t.Errorf("Expected source_pkg '/tmp/pkg.tgz', got '%s'", jsonBootData.SourcePkg)
	}
	if jsonBootData.TargetDir != "/www" {
		t.Errorf("Expected target_dir '/www', got '%s'", jsonBootData.TargetDir)
	}
	if !jsonBootData.LocalRepoAdded {
		t.Error("Expected local_repo_added to be true")
	}
	if len(jsonBootData.ExtractedFiles) != 2 {
		t.Errorf("Expected 2 extracted_files, got %d", len(jsonBootData.ExtractedFiles))
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlBootData RepoBootData
	if err := yaml.Unmarshal(yamlData, &yamlBootData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlBootData.LocalRepoPath != "/etc/yum.repos.d/local.repo" {
		t.Errorf("Unexpected local_repo_path: %s", yamlBootData.LocalRepoPath)
	}
}

func TestRepoBootDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoBootData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test without local repo
	dataNoLocal := &RepoBootData{
		SourcePkg:      "/tmp/pkg.tgz",
		TargetDir:      "/www",
		LocalRepoAdded: false,
	}
	text := dataNoLocal.Text()
	if !strings.Contains(text, "/tmp/pkg.tgz") {
		t.Errorf("Expected source_pkg in text, got '%s'", text)
	}
	if !strings.Contains(text, "/www") {
		t.Errorf("Expected target_dir in text, got '%s'", text)
	}

	// Test with local repo
	dataWithLocal := &RepoBootData{
		SourcePkg:      "/tmp/pkg.tgz",
		TargetDir:      "/www",
		LocalRepoAdded: true,
		LocalRepoPath:  "/etc/yum.repos.d/local.repo",
	}
	textLocal := dataWithLocal.Text()
	if !strings.Contains(textLocal, "local repo added") {
		t.Errorf("Expected 'local repo added' in text, got '%s'", textLocal)
	}
	if !strings.Contains(textLocal, "/etc/yum.repos.d/local.repo") {
		t.Errorf("Expected local_repo_path in text, got '%s'", textLocal)
	}
}

func TestRepoCacheDataSerialization(t *testing.T) {
	data := &RepoCacheData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		SourceDir:     "/www",
		TargetPkg:     "/tmp/pkg.tgz",
		IncludedRepos: []string{"pigsty", "mssql"},
		PackageSize:   1073741824, // 1 GiB
		PackageMD5:    "d41d8cd98f00b204e9800998ecf8427e",
		DurationMs:    60000,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"os_env"`) {
		t.Error("JSON should contain 'os_env' field")
	}
	if !strings.Contains(jsonStr, `"source_dir"`) {
		t.Error("JSON should contain 'source_dir' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"target_pkg"`) {
		t.Error("JSON should contain 'target_pkg' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"included_repos"`) {
		t.Error("JSON should contain 'included_repos' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"package_size"`) {
		t.Error("JSON should contain 'package_size' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"package_md5"`) {
		t.Error("JSON should contain 'package_md5' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field (snake_case)")
	}

	var jsonCacheData RepoCacheData
	if err := json.Unmarshal(jsonData, &jsonCacheData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonCacheData.SourceDir != "/www" {
		t.Errorf("Expected source_dir '/www', got '%s'", jsonCacheData.SourceDir)
	}
	if len(jsonCacheData.IncludedRepos) != 2 {
		t.Errorf("Expected 2 included_repos, got %d", len(jsonCacheData.IncludedRepos))
	}
	if jsonCacheData.PackageSize != 1073741824 {
		t.Errorf("Expected package_size 1073741824, got %d", jsonCacheData.PackageSize)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlCacheData RepoCacheData
	if err := yaml.Unmarshal(yamlData, &yamlCacheData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlCacheData.PackageMD5 != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("Unexpected package_md5: %s", yamlCacheData.PackageMD5)
	}
}

func TestRepoCacheDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoCacheData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test normal case
	data := &RepoCacheData{
		TargetPkg:     "/tmp/pkg.tgz",
		IncludedRepos: []string{"pigsty", "mssql"},
		PackageSize:   1073741824, // 1 GiB
	}
	text := data.Text()
	if !strings.Contains(text, "2 repos") {
		t.Errorf("Expected '2 repos' in text, got '%s'", text)
	}
	if !strings.Contains(text, "/tmp/pkg.tgz") {
		t.Errorf("Expected target_pkg in text, got '%s'", text)
	}
	if !strings.Contains(text, "GiB") {
		t.Errorf("Expected 'GiB' in text, got '%s'", text)
	}
}

func TestRepoReloadDataSerialization(t *testing.T) {
	data := &RepoReloadData{
		SourceURL:    "https://repo.pigsty.io/ext/data/repo.yml",
		RepoCount:    25,
		CatalogPath:  "/root/.pig/repo.yml",
		DownloadedAt: "2026-02-03T10:00:00Z",
		DurationMs:   1234,
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"source_url"`) {
		t.Error("JSON should contain 'source_url' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"repo_count"`) {
		t.Error("JSON should contain 'repo_count' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"catalog_path"`) {
		t.Error("JSON should contain 'catalog_path' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"downloaded_at"`) {
		t.Error("JSON should contain 'downloaded_at' field (snake_case)")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field (snake_case)")
	}

	var jsonReloadData RepoReloadData
	if err := json.Unmarshal(jsonData, &jsonReloadData); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if jsonReloadData.SourceURL != "https://repo.pigsty.io/ext/data/repo.yml" {
		t.Errorf("Expected source_url 'https://repo.pigsty.io/ext/data/repo.yml', got '%s'", jsonReloadData.SourceURL)
	}
	if jsonReloadData.RepoCount != 25 {
		t.Errorf("Expected repo_count 25, got %d", jsonReloadData.RepoCount)
	}

	// Test YAML serialization
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	var yamlReloadData RepoReloadData
	if err := yaml.Unmarshal(yamlData, &yamlReloadData); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if yamlReloadData.CatalogPath != "/root/.pig/repo.yml" {
		t.Errorf("Unexpected catalog_path: %s", yamlReloadData.CatalogPath)
	}
	if yamlReloadData.DownloadedAt != "2026-02-03T10:00:00Z" {
		t.Errorf("Unexpected downloaded_at: %s", yamlReloadData.DownloadedAt)
	}
}

func TestRepoReloadDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoReloadData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test with catalog path
	dataWithPath := &RepoReloadData{
		SourceURL:   "https://repo.pigsty.io/ext/data/repo.yml",
		RepoCount:   25,
		CatalogPath: "/root/.pig/repo.yml",
	}
	text := dataWithPath.Text()
	if !strings.Contains(text, "25 repos") {
		t.Errorf("Expected '25 repos' in text, got '%s'", text)
	}
	if !strings.Contains(text, "repo.pigsty.io") {
		t.Errorf("Expected source_url in text, got '%s'", text)
	}
	if !strings.Contains(text, "/root/.pig/repo.yml") {
		t.Errorf("Expected catalog_path in text, got '%s'", text)
	}

	// Test without catalog path
	dataNoPath := &RepoReloadData{
		SourceURL: "https://repo.pigsty.io/ext/data/repo.yml",
		RepoCount: 25,
	}
	textNoPath := dataNoPath.Text()
	if !strings.Contains(textNoPath, "25 repos") {
		t.Errorf("Expected '25 repos' in text, got '%s'", textNoPath)
	}
	if strings.Contains(textNoPath, " to ") {
		t.Errorf("Should not contain ' to ' without catalog path, got '%s'", textNoPath)
	}
}

func TestCreateReposWithResultUnsupportedOS(t *testing.T) {
	// On macOS, this should return unsupported OS error
	result := CreateReposWithResult([]string{"/www/pigsty"})
	if result == nil {
		t.Fatal("CreateReposWithResult should not return nil")
	}

	// On non-Linux systems, expect unsupported OS error
	if !result.Success {
		if result.Code == output.CodeRepoUnsupportedOS {
			t.Log("CreateReposWithResult correctly returns unsupported OS error on non-Linux")
		} else {
			t.Logf("CreateReposWithResult returned unexpected error code: %d - %s", result.Code, result.Message)
		}
	}
}

func TestRepoReloadDataOmitempty(t *testing.T) {
	// Test that optional fields are omitted when empty
	data := &RepoReloadData{
		SourceURL:  "https://repo.pigsty.io/ext/data/repo.yml",
		RepoCount:  25,
		DurationMs: 1234,
		// CatalogPath and DownloadedAt are empty/omitted
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonData)

	// These fields should NOT appear when empty (omitempty)
	if strings.Contains(jsonStr, `"catalog_path"`) {
		t.Error("JSON should not contain 'catalog_path' field when empty")
	}
	if strings.Contains(jsonStr, `"downloaded_at"`) {
		t.Error("JSON should not contain 'downloaded_at' field when empty")
	}

	// These required fields SHOULD appear
	if !strings.Contains(jsonStr, `"source_url"`) {
		t.Error("JSON should contain 'source_url' field")
	}
	if !strings.Contains(jsonStr, `"repo_count"`) {
		t.Error("JSON should contain 'repo_count' field")
	}
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Error("JSON should contain 'duration_ms' field")
	}
}

func TestBootWithResultPackageNotFound(t *testing.T) {
	// Test BootWithResult with non-existent package
	result := BootWithResult("/nonexistent/package.tgz", "/tmp/test-boot")
	if result == nil {
		t.Fatal("BootWithResult should not return nil")
	}

	// Should fail with package not found error
	if result.Success {
		t.Error("BootWithResult should fail for non-existent package")
	}

	if result.Code != output.CodeRepoPackageNotFound {
		t.Errorf("Expected code %d (CodeRepoPackageNotFound), got %d", output.CodeRepoPackageNotFound, result.Code)
	}

	// Verify data is attached even on failure
	if result.Data == nil {
		t.Error("Result.Data should not be nil even on failure")
	}

	bootData, ok := result.Data.(*RepoBootData)
	if !ok {
		t.Fatalf("Result.Data should be *RepoBootData, got %T", result.Data)
	}

	if bootData.SourcePkg != "/nonexistent/package.tgz" {
		t.Errorf("Expected source_pkg '/nonexistent/package.tgz', got '%s'", bootData.SourcePkg)
	}
	if bootData.DurationMs < 0 {
		t.Error("DurationMs should be non-negative")
	}
}

func TestCacheWithResultDirNotFound(t *testing.T) {
	// Test CacheWithResult with non-existent directory
	result := CacheWithResult("/nonexistent/dir", "/tmp/test-cache.tgz", []string{"pigsty"})
	if result == nil {
		t.Fatal("CacheWithResult should not return nil")
	}

	// Should fail with directory not found error
	if result.Success {
		t.Error("CacheWithResult should fail for non-existent directory")
	}

	if result.Code != output.CodeRepoDirNotFound {
		t.Errorf("Expected code %d (CodeRepoDirNotFound), got %d", output.CodeRepoDirNotFound, result.Code)
	}

	// Verify data is attached even on failure
	if result.Data == nil {
		t.Error("Result.Data should not be nil even on failure")
	}

	cacheData, ok := result.Data.(*RepoCacheData)
	if !ok {
		t.Fatalf("Result.Data should be *RepoCacheData, got %T", result.Data)
	}

	if cacheData.SourceDir != "/nonexistent/dir" {
		t.Errorf("Expected source_dir '/nonexistent/dir', got '%s'", cacheData.SourceDir)
	}
	if cacheData.DurationMs < 0 {
		t.Error("DurationMs should be non-negative")
	}
}

func TestBootWithResultDefaults(t *testing.T) {
	// Test that BootWithResult uses default values when empty strings are passed
	result := BootWithResult("", "")
	if result == nil {
		t.Fatal("BootWithResult should not return nil")
	}

	// Should fail (package doesn't exist) but with correct defaults
	bootData, ok := result.Data.(*RepoBootData)
	if !ok {
		t.Fatalf("Result.Data should be *RepoBootData, got %T", result.Data)
	}

	// Check defaults are applied
	if bootData.SourcePkg != "/tmp/pkg.tgz" {
		t.Errorf("Expected default source_pkg '/tmp/pkg.tgz', got '%s'", bootData.SourcePkg)
	}
	if bootData.TargetDir != "/www" {
		t.Errorf("Expected default target_dir '/www', got '%s'", bootData.TargetDir)
	}
}

func TestCacheWithResultDefaults(t *testing.T) {
	// Test that CacheWithResult uses default values when empty/nil are passed
	result := CacheWithResult("", "", nil)
	if result == nil {
		t.Fatal("CacheWithResult should not return nil")
	}

	// Should fail (directory doesn't exist) but with correct defaults
	cacheData, ok := result.Data.(*RepoCacheData)
	if !ok {
		t.Fatalf("Result.Data should be *RepoCacheData, got %T", result.Data)
	}

	// Check defaults are applied
	if cacheData.SourceDir != "/www" {
		t.Errorf("Expected default source_dir '/www', got '%s'", cacheData.SourceDir)
	}
	if cacheData.TargetPkg != "/tmp/pkg.tgz" {
		t.Errorf("Expected default target_pkg '/tmp/pkg.tgz', got '%s'", cacheData.TargetPkg)
	}
	if len(cacheData.IncludedRepos) != 1 || cacheData.IncludedRepos[0] != "pigsty" {
		t.Errorf("Expected default included_repos ['pigsty'], got %v", cacheData.IncludedRepos)
	}
}

/********************
 * Tests for DTO Text() methods (Story 7.2)
 ********************/

func TestRepoListDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoListData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test available repos (showAll=false)
	data := &RepoListData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		ShowAll:   false,
		RepoCount: 2,
		Repos: []*RepoSummary{
			{
				Name:        "pigsty-pgsql",
				Description: "Pigsty PGSQL",
				Module:      "pgsql",
				Releases:    []int{8, 9, 10},
				Arch:        []string{"x86_64", "aarch64"},
				BaseURL:     "https://repo.pigsty.io/yum/pgsql",
				Available:   true,
			},
			{
				Name:        "pgdg17",
				Description: "PGDG 17",
				Module:      "pgdg",
				Releases:    []int{8, 9, 10},
				Arch:        []string{"x86_64", "aarch64"},
				BaseURL:     "https://download.postgresql.org/pub/repos/yum",
				Available:   true,
			},
		},
		Modules: map[string][]string{
			"pgsql":  {"pigsty-pgsql", "pgdg17"},
			"pigsty": {"pigsty-pgsql"},
		},
	}

	text := data.Text()
	if !strings.Contains(text, "os_environment:") {
		t.Errorf("Expected os_environment header, got '%s'", text)
	}
	if !strings.Contains(text, "el9") {
		t.Errorf("Expected 'el9' in text, got '%s'", text)
	}
	if !strings.Contains(text, "repo_upstream:") {
		t.Errorf("Expected 'repo_upstream:' in text, got '%s'", text)
	}
	if !strings.Contains(text, "pigsty-pgsql") {
		t.Errorf("Expected 'pigsty-pgsql' in text, got '%s'", text)
	}
	if !strings.Contains(text, "repo_modules:") {
		t.Errorf("Expected 'repo_modules:' in text, got '%s'", text)
	}

	// Test showAll=true
	dataAll := &RepoListData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		ShowAll:   true,
		RepoCount: 2,
		Repos: []*RepoSummary{
			{
				Name:      "pigsty-pgsql",
				Available: true,
			},
			{
				Name:      "some-unavailable",
				Available: false,
			},
		},
	}
	textAll := dataAll.Text()
	if !strings.Contains(textAll, "repo_rawdata:") {
		t.Errorf("Expected 'repo_rawdata:' for showAll, got '%s'", textAll)
	}
	// Check markers
	if !strings.Contains(textAll, "o {") {
		t.Errorf("Expected 'o' marker for available repo, got '%s'", textAll)
	}
	if !strings.Contains(textAll, "x {") {
		t.Errorf("Expected 'x' marker for unavailable repo, got '%s'", textAll)
	}
}

func TestRepoInfoDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoInfoData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test with repos
	data := &RepoInfoData{
		Requested: []string{"pigsty"},
		Repos: []*RepoDetail{
			{
				Name:        "pigsty-pgsql",
				Description: "Pigsty PGSQL Repo",
				Module:      "pgsql",
				Releases:    []int{8, 9, 10},
				Arch:        []string{"x86_64", "aarch64"},
				BaseURL: map[string]string{
					"default": "https://repo.pigsty.io/yum/pgsql",
					"china":   "https://repo.pigsty.cc/yum/pgsql",
				},
				Meta: map[string]string{
					"enabled":  "1",
					"gpgcheck": "0",
				},
				Available: true,
				Content:   "[pigsty-pgsql]\nname=pigsty-pgsql\n",
			},
		},
	}

	text := data.Text()
	if !strings.Contains(text, "#---------") {
		t.Errorf("Expected separator in text, got '%s'", text)
	}
	if !strings.Contains(text, "Name       : pigsty-pgsql") {
		t.Errorf("Expected 'Name       : pigsty-pgsql' in text, got '%s'", text)
	}
	if !strings.Contains(text, "Available  : Yes") {
		t.Errorf("Expected 'Available  : Yes' in text, got '%s'", text)
	}
	if !strings.Contains(text, "Module     : pgsql") {
		t.Errorf("Expected 'Module     : pgsql' in text, got '%s'", text)
	}
	if !strings.Contains(text, "china") {
		t.Errorf("Expected 'china' region in text, got '%s'", text)
	}
	if !strings.Contains(text, "# default repo content") {
		t.Errorf("Expected repo content section in text, got '%s'", text)
	}

	// Test unavailable repo
	unavailData := &RepoInfoData{
		Requested: []string{"test"},
		Repos: []*RepoDetail{
			{
				Name:      "test-repo",
				Available: false,
			},
		},
	}
	unavailText := unavailData.Text()
	if !strings.Contains(unavailText, "Available  : No") {
		t.Errorf("Expected 'Available  : No' in text, got '%s'", unavailText)
	}
}

func TestRepoStatusDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoStatusData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test with data
	data := &RepoStatusData{
		OSType:      "rpm",
		RepoDir:     "/etc/yum.repos.d",
		RepoFiles:   []string{"/etc/yum.repos.d/pigsty.repo", "/etc/yum.repos.d/pgdg.repo"},
		ActiveRepos: []string{"pigsty-pgsql", "pgdg17"},
	}

	text := data.Text()
	if !strings.Contains(text, "Repo Dir: /etc/yum.repos.d") {
		t.Errorf("Expected repo dir in text, got '%s'", text)
	}
	if !strings.Contains(text, "Repo Files:") {
		t.Errorf("Expected 'Repo Files:' in text, got '%s'", text)
	}
	if !strings.Contains(text, "pigsty.repo") {
		t.Errorf("Expected 'pigsty.repo' in text, got '%s'", text)
	}
	if !strings.Contains(text, "Active Repos:") {
		t.Errorf("Expected 'Active Repos:' in text, got '%s'", text)
	}
	if !strings.Contains(text, "pgdg17") {
		t.Errorf("Expected 'pgdg17' in text, got '%s'", text)
	}

	// Test with empty files
	emptyData := &RepoStatusData{
		OSType:    "deb",
		RepoDir:   "/etc/apt/sources.list.d",
		RepoFiles: []string{},
	}
	emptyText := emptyData.Text()
	if !strings.Contains(emptyText, "(none)") {
		t.Errorf("Expected '(none)' for empty repo files, got '%s'", emptyText)
	}
}

func TestRepoAddDataText(t *testing.T) {
	// Test nil receiver
	var nilData *RepoAddData
	if nilData.Text() != "" {
		t.Error("Text on nil should return empty string")
	}

	// Test with backup and added repos
	data := &RepoAddData{
		OSEnv: &OSEnvironment{
			Code:  "el9",
			Arch:  "x86_64",
			Type:  "rpm",
			Major: 9,
		},
		Region:           "default",
		RequestedModules: []string{"all"},
		ExpandedModules:  []string{"node", "pgsql"},
		AddedRepos: []*AddedRepoItem{
			{
				Module:   "node",
				FilePath: "/etc/yum.repos.d/node.repo",
				Repos:    []string{"baseos", "appstream"},
			},
			{
				Module:   "pgsql",
				FilePath: "/etc/yum.repos.d/pgsql.repo",
				Repos:    []string{"pigsty-pgsql", "pgdg17"},
			},
		},
		BackupInfo: &BackupInfo{
			BackupDir:     "/etc/yum.repos.d/backup",
			BackedUpFiles: []string{"/etc/yum.repos.d/old.repo"},
		},
		UpdateResult: &UpdateCacheResult{
			Command: "dnf makecache",
			Success: true,
		},
		DurationMs: 1234,
	}

	text := data.Text()
	if !strings.Contains(text, "Backed up 1 files") {
		t.Errorf("Expected backup info in text, got '%s'", text)
	}
	if !strings.Contains(text, "Added module: node") {
		t.Errorf("Expected 'Added module: node' in text, got '%s'", text)
	}
	if !strings.Contains(text, "Added module: pgsql") {
		t.Errorf("Expected 'Added module: pgsql' in text, got '%s'", text)
	}
	if !strings.Contains(text, "Cache updated: dnf makecache") {
		t.Errorf("Expected cache update info in text, got '%s'", text)
	}

	// Test with failed modules
	failedData := &RepoAddData{
		AddedRepos: []*AddedRepoItem{},
		Failed: []*FailedRepoItem{
			{
				Module: "nonexistent",
				Error:  "module not found",
				Code:   output.CodeRepoModuleNotFound,
			},
		},
	}
	failedText := failedData.Text()
	if !strings.Contains(failedText, "Failed module: nonexistent") {
		t.Errorf("Expected failed module info in text, got '%s'", failedText)
	}

	// Test with failed cache update
	failCacheData := &RepoAddData{
		UpdateResult: &UpdateCacheResult{
			Command: "apt-get update",
			Success: false,
			Error:   "network error",
		},
	}
	failCacheText := failCacheData.Text()
	if !strings.Contains(failCacheText, "Cache update failed") {
		t.Errorf("Expected cache update failure in text, got '%s'", failCacheText)
	}
}

func TestFormatIntSlice(t *testing.T) {
	tests := []struct {
		input    []int
		expected string
	}{
		{nil, "[]"},
		{[]int{}, "[]"},
		{[]int{9}, "[9]"},
		{[]int{8, 9, 10}, "[8,9,10]"},
	}
	for _, tt := range tests {
		result := formatIntSlice(tt.input)
		if result != tt.expected {
			t.Errorf("formatIntSlice(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatStrSlice(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{nil, "[]"},
		{[]string{}, "[]"},
		{[]string{"x86_64"}, "[x86_64]"},
		{[]string{"x86_64", "aarch64"}, "[x86_64, aarch64]"},
	}
	for _, tt := range tests {
		result := formatStrSlice(tt.input)
		if result != tt.expected {
			t.Errorf("formatStrSlice(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
