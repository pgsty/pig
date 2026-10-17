package repo

import (
	"os"
	"path/filepath"
	"testing"

	"pig/internal/config"
)

func TestGetRepoInfoUsesCurrentPlatformDefinition(t *testing.T) {
	oldType, oldCode, oldArch, oldDir := config.OSType, config.OSCode, config.OSArch, config.ConfigDir
	t.Cleanup(func() {
		config.OSType, config.OSCode, config.OSArch, config.ConfigDir = oldType, oldCode, oldArch, oldDir
	})
	config.ConfigDir = t.TempDir()
	fixture := `- name: selected
  description: EL definition
  releases: [9]
  arch: [aarch64, x86_64]
  baseurl: {default: 'https://example.invalid/yum/el9'}
- name: selected
  description: Debian definition
  releases: [12]
  arch: [aarch64, x86_64]
  baseurl: {default: 'deb https://example.invalid/deb bookworm main'}
`
	if err := os.WriteFile(filepath.Join(config.ConfigDir, "repo.yml"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ typ, code, arch, want string }{
		{config.DistroEL, "el9", "arm64", "EL definition"},
		{config.DistroDEB, "d12", "arm64", "Debian definition"},
		{config.DistroDEB, "d12", "amd64", "Debian definition"},
	} {
		t.Run(tc.code+tc.arch, func(t *testing.T) {
			config.OSType, config.OSCode, config.OSArch = tc.typ, tc.code, tc.arch
			result := GetRepoInfo([]string{"selected"})
			if !result.Success {
				t.Fatalf("repo info failed: %+v", result)
			}
			data := result.Data.(*RepoInfoData)
			if len(data.Repos) != 1 || data.Repos[0].Description != tc.want || !data.Repos[0].Available {
				t.Fatalf("wrong platform: %+v", data.Repos)
			}
		})
	}
}
