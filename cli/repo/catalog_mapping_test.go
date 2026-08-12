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

	assertRepo("nginx", "Nginx Repo", "rpm", "x86_64", []int{7, 8, 9, 10}, "")
	assertRepo("docker-ce", "Docker CE", "rpm", "x86_64", []int{7, 8, 9, 10}, "")
	assertRepo("timescaledb", "TimescaleDB", "rpm", "aarch64", []int{7, 8, 9, 10}, "")
	assertRepo("grafana", "Grafana", "rpm", "x86_64", []int{7, 8, 9, 10}, "")
	assertRepo("pigsty-pgsql", "Pigsty PGSQL", "rpm", "aarch64", []int{7, 8, 9, 10}, "")
	assertRepo("base", "EL 7 Base", "rpm", "x86_64", []int{7}, "https://archive.kernel.org/centos-vault/7.9.2009/os/$basearch/")
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
		if slices.Contains([]string{"mysql", "redis", "kubernetes"}, r.Name) && slices.Contains(r.Releases, 7) {
			t.Fatalf("removed optional repository %s must not advertise EL7: %+v", r.Name, r)
		}
		if r.Name == "pgdg19-beta" && slices.Contains(r.Releases, 8) && slicesContains(r.Arch, "aarch64") {
			t.Fatalf("pgdg19-beta must not advertise unavailable EL8 aarch64 metadata: %+v", r)
		}
	}
}

func TestEmbeddedEL7SpecialGroupBetweenELAndDEB(t *testing.T) {
	catalog := string(embedRepoData)
	elStart := strings.Index(catalog, "# EL REPOS (7/8/9/10)")
	specialStart := strings.Index(catalog, "# EL 7 SPECIAL REPOS (VIRTUAL GROUP)")
	debStart := strings.Index(catalog, "# DEB 11/12/13 Ubuntu 22/24/26 REPOS")
	if elStart < 0 || specialStart < 0 || debStart < 0 || !(elStart < specialStart && specialStart < debStart) {
		t.Fatalf("repository group order must be shared EL, special EL7, then DEB: el=%d special=%d deb=%d", elStart, specialStart, debStart)
	}

	var special []Repository
	if err := yaml.Unmarshal([]byte(catalog[specialStart:debStart]), &special); err != nil {
		t.Fatalf("failed to parse special EL7 group: %v", err)
	}
	if len(special) != 6 {
		t.Fatalf("special EL7 group contains %d repositories, want 6", len(special))
	}
	for _, r := range special {
		if len(r.Releases) != 1 || r.Releases[0] != 7 {
			t.Fatalf("special group contains a non-EL7 repository: %+v", r)
		}
		if _, ok := r.Meta["module_hotfixes"]; ok {
			t.Fatalf("EL7 repository must not use module_hotfixes: %+v", r)
		}
	}

	var shared []Repository
	if err := yaml.Unmarshal([]byte(catalog[elStart:specialStart]), &shared); err != nil {
		t.Fatalf("failed to parse shared EL group: %v", err)
	}
	sharedEL7 := make(map[string]bool)
	for _, r := range shared {
		if slices.Contains(r.Releases, 7) {
			sharedEL7[r.Name] = true
			if !slices.Equal(r.Releases, []int{7, 8, 9, 10}) {
				t.Fatalf("shared EL7 repository %s has releases %v, want [7 8 9 10]", r.Name, r.Releases)
			}
		}
	}
	for _, name := range []string{
		"pigsty-local", "pigsty-infra", "pigsty-pgsql", "nginx", "docker-ce",
		"pgdg-common", "pgdg14", "pgdg15", "pgdg-extras", "pgdg14-nonfree", "pgdg15-nonfree",
		"timescaledb", "grafana",
	} {
		if !sharedEL7[name] {
			t.Errorf("repository %s should advertise EL7 in the shared EL record", name)
		}
		delete(sharedEL7, name)
	}
	if len(sharedEL7) != 0 {
		t.Fatalf("unexpected repositories advertise EL7 in the shared EL group: %v", sharedEL7)
	}
}

func TestEmbeddedEL7CatalogMatchesSelectedPigstyRepositories(t *testing.T) {
	var repos []Repository
	if err := yaml.Unmarshal(embedRepoData, &repos); err != nil {
		t.Fatalf("failed to parse embedded repo catalog: %v", err)
	}

	el7 := make(map[string]Repository)
	for _, r := range repos {
		if slices.Contains(r.Releases, 7) {
			el7[r.Name] = r
		}
	}
	want := []struct {
		name, description, module string
	}{
		{"pigsty-local", "Pigsty Local", "local"}, {"pigsty-infra", "Pigsty INFRA", "infra"}, {"pigsty-pgsql", "Pigsty PGSQL", "pgsql"},
		{"nginx", "Nginx Repo", "nginx"}, {"docker-ce", "Docker CE", "docker"},
		{"base", "EL 7 Base", "node"}, {"updates", "EL 7 Updates", "node"}, {"extras", "EL 7 Extras", "node"}, {"epel", "EL 7 EPEL", "node"},
		{"centos-sclo", "EL 7 SCLo", "node"}, {"centos-sclo-rh", "EL 7 SCLo rh", "node"},
		{"pgdg-common", "PostgreSQL Common", "pgsql"}, {"pgdg14", "PostgreSQL 14", "pgsql"}, {"pgdg15", "PostgreSQL 15", "pgsql"},
		{"pgdg-extras", "PostgreSQL Extra", "extra"}, {"pgdg14-nonfree", "PostgreSQL 14+", "extra"}, {"pgdg15-nonfree", "PostgreSQL 15+", "extra"},
		{"timescaledb", "TimescaleDB", "extra"}, {"grafana", "Grafana", "grafana"},
	}
	if len(el7) != len(want) {
		t.Fatalf("EL7 catalog contains %d unique repositories, want %d selected from Pigsty el7.x86_64.yml", len(el7), len(want))
	}
	for _, item := range want {
		r, ok := el7[item.name]
		if !ok {
			t.Errorf("Pigsty EL7 repository %s is missing", item.name)
			continue
		}
		if r.Module != item.module {
			t.Errorf("EL7 repository %s module = %q, want PIG CLI module %q", item.name, r.Module, item.module)
		}
		if r.Description != item.description {
			t.Errorf("EL7 repository %s description = %q, want Pigsty description %q", item.name, r.Description, item.description)
		}
		if !slices.Contains(r.Arch, "x86_64") {
			t.Errorf("EL7 repository %s does not support Pigsty's x86_64 platform: %v", item.name, r.Arch)
		}
	}

	// PIG-specific differences from Pigsty's Ansible vars are intentional:
	// local is a file repo, and nginx/docker remain independently selectable modules.
	if got := el7["pigsty-local"].BaseURL["default"]; got != "file:///www/pigsty" {
		t.Errorf("PIG local EL7 repo = %q, want file:///www/pigsty", got)
	}
	for _, name := range []string{"base", "updates", "extras", "centos-sclo", "centos-sclo-rh"} {
		if !strings.HasPrefix(el7[name].BaseURL["default"], "https://archive.kernel.org/centos-vault/7.9.2009/") {
			t.Errorf("EL7 repository %s must use the reachable CentOS 7.9.2009 archive: %q", name, el7[name].BaseURL["default"])
		}
	}
	if !strings.Contains(el7["epel"].BaseURL["china"], "/epel-archive/") {
		t.Errorf("EL7 EPEL China mirror must use the archive tree: %q", el7["epel"].BaseURL["china"])
	}
	for _, name := range []string{"pgdg14-nonfree", "pgdg15-nonfree"} {
		if el7[name].Meta["skip_if_unavailable"] != "1" {
			t.Errorf("EL7 repository %s must preserve Pigsty skip_if_unavailable", name)
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
