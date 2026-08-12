package repo

import (
	"slices"
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
	assertReleases("timescaledb", "TimescaleDB", "deb", "x86_64", []int{11, 12, 13, 22, 24, 26})
	assertReleases("redis", "Redis", "deb", "x86_64", []int{11, 12, 22, 24, 26})
	assertReleases("clickhouse", "ClickHouse", "deb", "x86_64", []int{11, 12, 13, 22, 24, 26})
	assertReleases("mysql", "MySQL 8.4 LTS", "deb", "x86_64", []int{12, 13, 22, 24})

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

func TestCurrentThirdPartyRepoPolicy(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	assertRepo := func(name, desc, distro, arch string, wantReleases []int, wantURL string) {
		t.Helper()
		for _, r := range repos {
			if r.Name != name || r.Description != desc || r.InferOS() != distro || !slicesContains(r.Arch, arch) {
				continue
			}
			if compactIntArray(r.Releases) != compactIntArray(wantReleases) {
				t.Fatalf("%s/%s/%s/%s releases = %v, want %v", name, desc, distro, arch, r.Releases, wantReleases)
			}
			if wantURL != "" && r.BaseURL["default"] != wantURL {
				t.Fatalf("%s/%s default URL = %q, want %q", name, desc, r.BaseURL["default"], wantURL)
			}
			return
		}
		t.Fatalf("repo %s/%s/%s/%s not found", name, desc, distro, arch)
	}

	assertRepo("timescaledb", "TimescaleDB", "rpm", "aarch64", []int{8, 9, 10}, "")
	assertRepo("pigsty-pgsql", "Pigsty PGSQL", "rpm", "aarch64", []int{8, 9, 10}, "")
	assertRepo("pgdg19-beta", "PostgreSQL 19b", "rpm", "x86_64", []int{8, 9, 10}, "")
	assertRepo("pgdg19-beta", "PostgreSQL 19b", "rpm", "aarch64", []int{9, 10}, "")
	assertRepo("mongo", "MongoDB", "rpm", "x86_64", []int{8, 9, 10}, "")
	assertRepo("gitlab-ee", "Gitlab EE", "rpm", "x86_64", []int{8, 9, 10}, "")
	assertRepo("gitlab-ce", "Gitlab CE", "rpm", "aarch64", []int{8, 9, 10}, "")
	assertRepo("kubernetes", "Kubernetes", "rpm", "aarch64", []int{8, 9, 10}, "https://pkgs.k8s.io/core:/stable:/v1.36/rpm/")
	assertRepo("kubernetes", "Kubernetes", "deb", "aarch64", []int{11, 12, 13, 22, 24, 26}, "https://pkgs.k8s.io/core:/stable:/v1.36/deb/ /")

	for _, r := range repos {
		if r.Name == "grafana" && r.InferOS() == "rpm" && r.BaseURL["china"] != "https://mirrors.cloud.tencent.com/grafana/yum/rpm/" {
			t.Fatalf("Grafana RPM China mirror = %q", r.BaseURL["china"])
		}
		if slices.Contains(r.Releases, 7) {
			t.Fatalf("embedded repository catalog must not advertise EL7: %+v", r)
		}
		if r.Name == "pgdg19-beta" && slices.Contains(r.Releases, 8) && slicesContains(r.Arch, "aarch64") {
			t.Fatalf("pgdg19-beta must not advertise unavailable EL8 aarch64 metadata: %+v", r)
		}
	}
}

func TestPerconaTDERepoContract(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	want := map[string]string{
		"rpm": "https://repo.percona.com/ppg-18.4/yum/release/$releasever/RPMS/$basearch",
		"deb": "https://repo.percona.com/ppg-18.4/apt ${distro_codename} main",
	}
	seen := make(map[string]bool)
	for _, r := range repos {
		if r.Name != "percona" || r.Description != "Percona TDE" {
			continue
		}
		distro := r.InferOS()
		if r.BaseURL["default"] != want[distro] {
			t.Fatalf("percona %s repository = %q, want %q", distro, r.BaseURL["default"], want[distro])
		}
		seen[distro] = true
	}
	for _, distro := range []string{"rpm", "deb"} {
		if !seen[distro] {
			t.Fatalf("percona %s repository not found", distro)
		}
	}
}

func TestPGDGChinaMirrorContract(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	var yumCount int
	var aptStable, aptBeta bool
	for _, r := range repos {
		defaultURL := r.BaseURL["default"]
		chinaURL := r.BaseURL["china"]
		switch {
		case strings.HasPrefix(defaultURL, "https://download.postgresql.org/pub/repos/yum/"):
			// China RPM entries are an ordered failover list: Tencent first,
			// repo.pigsty.cc as the compatibility fallback.
			path := strings.TrimPrefix(defaultURL, "https://download.postgresql.org/pub/repos/yum/")
			candidates := strings.Fields(chinaURL)
			if len(candidates) != 2 {
				t.Fatalf("%s China mirror = %q, want 2 failover candidates", r.Name, chinaURL)
			}
			wantPrimary := "https://mirrors.cloud.tencent.com/postgresql/repos/yum/" + path
			if candidates[0] != wantPrimary {
				t.Fatalf("%s China primary = %q, want %q", r.Name, candidates[0], wantPrimary)
			}
			wantFallback := "https://repo.pigsty.cc/yum/pgdg/" + path
			if candidates[1] != wantFallback {
				t.Fatalf("%s China fallback = %q, want %q", r.Name, candidates[1], wantFallback)
			}
			yumCount++
		case r.Name == "pgdg" && strings.Contains(defaultURL, "apt.postgresql.org"):
			want := "https://mirrors.cloud.tencent.com/postgresql/repos/apt/ ${distro_codename}-pgdg main"
			if chinaURL != want {
				t.Fatalf("PGDG APT China mirror = %q, want %q", chinaURL, want)
			}
			aptStable = true
		case r.Name == "pgdg-beta" && strings.Contains(defaultURL, "apt.postgresql.org"):
			want := "https://mirrors.cloud.tencent.com/postgresql/repos/apt/ ${distro_codename}-pgdg-testing main 19"
			if chinaURL != want {
				t.Fatalf("PGDG beta APT China mirror = %q, want %q", chinaURL, want)
			}
			aptBeta = true
		}
	}

	if yumCount == 0 || !aptStable || !aptBeta {
		t.Fatalf("incomplete PGDG mirror coverage: yum=%d apt-stable=%t apt-beta=%t", yumCount, aptStable, aptBeta)
	}
}

func TestPGDGMirrorRouteSelectsChinaDefinitions(t *testing.T) {
	m := &Manager{Region: "default"}
	applyPGDGMirrorRoute(m)
	if m.Region != "china" {
		t.Fatalf("PGDG mirror route region = %q, want china", m.Region)
	}

	// The selector only changes the region; catalog definitions remain the
	// authoritative source for Tencent-first ordering and compatibility fallbacks.
	applyPGDGMirrorRoute(nil)
}

func TestPXB84ChinaMirrorContract(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	want := map[string]string{
		"rpm": "https://mirrors.cloud.tencent.com/percona/pxb-84-lts/yum/release/$releasever/RPMS/$basearch/",
		"deb": "https://mirrors.cloud.tencent.com/percona/pxb-84-lts/apt ${distro_codename} main",
	}
	seen := make(map[string]bool)
	for _, r := range repos {
		if r.Name != "pxb84" {
			continue
		}
		distro := r.InferOS()
		if r.BaseURL["china"] != want[distro] {
			t.Fatalf("pxb84 %s China mirror = %q, want %q", distro, r.BaseURL["china"], want[distro])
		}
		if r.GetBaseURL("china") != want[distro] {
			t.Fatalf("pxb84 %s does not select its China mirror", distro)
		}
		seen[distro] = true
	}
	for _, distro := range []string{"rpm", "deb"} {
		if !seen[distro] {
			t.Fatalf("pxb84 %s repository not found", distro)
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
