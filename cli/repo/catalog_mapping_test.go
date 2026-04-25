package repo

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUbuntuRepoChannelMapping(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	expectedSuffix := map[string]string{
		"updates":   "-updates",
		"backports": "-backports",
		"security":  "-security",
	}

	// For Ubuntu, we expect two entries (x86_64 + aarch64) for each channel.
	channelCount := map[string]int{
		"updates":   0,
		"backports": 0,
		"security":  0,
	}

	for _, r := range repos {
		if !strings.HasPrefix(r.Description, "Ubuntu ") {
			continue
		}
		suffix, ok := expectedSuffix[r.Name]
		if !ok {
			continue
		}

		channelCount[r.Name]++

		defURL := r.BaseURL["default"]
		chinaURL := r.BaseURL["china"]
		wantFragment := "${distro_codename}" + suffix

		if !strings.Contains(defURL, wantFragment) {
			t.Errorf("ubuntu repo %q default URL mismatch: got %q, want fragment %q", r.Name, defURL, wantFragment)
		}
		if chinaURL != "" && !strings.Contains(chinaURL, wantFragment) {
			t.Errorf("ubuntu repo %q china URL mismatch: got %q, want fragment %q", r.Name, chinaURL, wantFragment)
		}
	}

	for channel, count := range channelCount {
		if count != 2 {
			t.Errorf("expected 2 ubuntu %q entries (x86_64 + aarch64), got %d", channel, count)
		}
	}
}

func TestDebianRepoComponentMapping(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	expectedSuffix := map[string]string{
		"base":     "",
		"updates":  "-updates",
		"security": "-security",
	}
	channelCount := map[string]int{
		"base":     0,
		"updates":  0,
		"security": 0,
	}
	bannedComponents := []string{"restricted", "universe", "multiverse"}

	for _, r := range repos {
		if !strings.HasPrefix(r.Description, "Debian ") {
			continue
		}

		suffix, ok := expectedSuffix[r.Name]
		if !ok {
			continue
		}
		channelCount[r.Name]++

		defURL := r.BaseURL["default"]
		chinaURL := r.BaseURL["china"]
		wantFragment := "${distro_codename}" + suffix

		if !strings.Contains(defURL, wantFragment) {
			t.Errorf("debian repo %q default URL mismatch: got %q, want fragment %q", r.Name, defURL, wantFragment)
		}
		if chinaURL == "" || !strings.Contains(chinaURL, wantFragment) {
			t.Errorf("debian repo %q china URL mismatch: got %q, want fragment %q", r.Name, chinaURL, wantFragment)
		}

		defComponents := aptComponents(defURL)
		chinaComponents := aptComponents(chinaURL)
		if len(defComponents) == 0 || len(chinaComponents) == 0 {
			t.Errorf("debian repo %q failed to parse apt components, default=%q, china=%q", r.Name, defURL, chinaURL)
			continue
		}
		if strings.Join(defComponents, " ") != strings.Join(chinaComponents, " ") {
			t.Errorf("debian repo %q component mismatch: default=%q china=%q", r.Name, strings.Join(defComponents, " "), strings.Join(chinaComponents, " "))
		}

		for _, c := range bannedComponents {
			if slicesContains(defComponents, c) || slicesContains(chinaComponents, c) {
				t.Errorf("debian repo %q should not contain ubuntu component %q: default=%q china=%q", r.Name, c, defURL, chinaURL)
			}
		}
	}

	for channel, count := range channelCount {
		if count != 1 {
			t.Errorf("expected 1 debian %q entry, got %d", channel, count)
		}
	}
}

func TestUbuntuRepoReleasePolicy(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	assertReleases := func(name, desc, distro, arch string, want []int) {
		t.Helper()
		for _, r := range repos {
			if r.Name == name && r.Description == desc && r.InferOS() == distro && slicesContains(r.Arch, arch) {
				if compactIntArray(r.Releases) != compactIntArray(want) {
					t.Fatalf("%s/%s/%s releases = %v, want %v", name, desc, arch, r.Releases, want)
				}
				return
			}
		}
		t.Fatalf("repo %s/%s/%s/%s not found", name, desc, distro, arch)
	}

	assertReleases("base", "Ubuntu Basic", "deb", "x86_64", []int{22, 24, 26})
	assertReleases("base", "Ubuntu Basic", "deb", "aarch64", []int{22, 24, 26})
	assertReleases("pgdg", "PGDG", "deb", "x86_64", []int{11, 12, 13, 22, 24, 26})
	assertReleases("haproxyu", "Haproxy Ubuntu", "deb", "x86_64", []int{24, 26})
	assertReleases("timescaledb", "TimescaleDB", "deb", "x86_64", []int{11, 12, 13, 22, 24})
	assertReleases("clickhouse", "ClickHouse", "deb", "x86_64", []int{11, 12, 13, 22, 24, 26})
	assertReleases("mysql", "MySQL", "deb", "x86_64", []int{11, 12, 22, 24})

	for _, r := range repos {
		if r.Name == "wiltondb" && r.InferOS() == "deb" {
			t.Fatalf("wiltondb should not be present in deb repo catalog anymore: %+v", r)
		}
		if r.Name == "percona" && r.Description == "Percona TDE" && r.InferOS() == "deb" {
			if _, ok := r.BaseURL["origin"]; ok {
				t.Fatalf("percona deb repo should not carry origin mirror metadata anymore: %+v", r.BaseURL)
			}
		}
	}
}

func aptComponents(repoURL string) []string {
	fields := strings.Fields(repoURL)
	if len(fields) < 3 {
		return nil
	}
	return fields[2:]
}

func slicesContains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
