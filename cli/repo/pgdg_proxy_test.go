package repo

import "testing"

func TestApplyPGDGProxyRouteRewritesOnlyPGDGProxyURLs(t *testing.T) {
	pgdgYum := &Repository{
		Name: "pgdg18",
		BaseURL: map[string]string{
			"default": "https://download.postgresql.org/pub/repos/yum/18/redhat/rhel-$releasever-$basearch",
			"china":   "https://mirrors.aliyun.com/postgresql/repos/yum/18/redhat/rhel-$releasever-$basearch",
		},
	}
	pgdgApt := &Repository{
		Name: "pgdg",
		BaseURL: map[string]string{
			"default": "http://apt.postgresql.org/pub/repos/apt/ ${distro_codename}-pgdg main",
			"china":   "https://mirrors.aliyun.com/postgresql/repos/apt/ ${distro_codename}-pgdg main",
		},
	}
	docker := &Repository{
		Name: "docker-ce",
		BaseURL: map[string]string{
			"default": "https://download.docker.com/linux/centos/$releasever/$basearch/stable",
			"china":   "https://mirrors.aliyun.com/docker-ce/linux/centos/$releasever/$basearch/stable",
		},
	}
	m := &Manager{
		Region: "default",
		Data:   []*Repository{pgdgYum, pgdgApt, docker},
	}

	applyPGDGProxyRoute(m)

	if m.Region != "china" {
		t.Fatalf("PGDG proxy route should force proxy region, got %q", m.Region)
	}
	if got, want := pgdgYum.BaseURL["china"], "http://beta.pigsty.cc/yum/pgdg/18/redhat/rhel-$releasever-$basearch"; got != want {
		t.Fatalf("pgdg yum proxy URL = %q, want %q", got, want)
	}
	if got, want := pgdgApt.BaseURL["china"], "http://beta.pigsty.cc/apt/pgdg/ ${distro_codename}-pgdg main"; got != want {
		t.Fatalf("pgdg apt proxy URL = %q, want %q", got, want)
	}
	if got, want := docker.BaseURL["china"], "https://mirrors.aliyun.com/docker-ce/linux/centos/$releasever/$basearch/stable"; got != want {
		t.Fatalf("non-PGDG proxy URL = %q, want unchanged %q", got, want)
	}
}
