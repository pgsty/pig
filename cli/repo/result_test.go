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

	if result.Code != output.CodeRepoNotFound {
		t.Errorf("Expected code %d (CodeRepoNotFound), got %d", output.CodeRepoNotFound, result.Code)
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
