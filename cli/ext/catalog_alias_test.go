package ext

import (
	"fmt"
	"strings"
	"testing"

	"pig/internal/config"
)

func withAliasMapTestEnv(t *testing.T, osType, osCode, osArch string) func() {
	t.Helper()

	oldOSType := config.OSType
	oldOSCode := config.OSCode
	oldOSArch := config.OSArch

	config.OSType = osType
	config.OSCode = osCode
	config.OSArch = osArch

	return func() {
		config.OSType = oldOSType
		config.OSCode = oldOSCode
		config.OSArch = oldOSArch
	}
}

func TestLoadAliasMapRequestedAliasesEL9(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroEL, "el9", "amd64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)

	expect := map[string]string{
		"docker":         "docker-ce docker-compose-plugin",
		"kafka-stack":    "kafka kafka-exporter jmx-exporter",
		"mysql":          "mysql-community-server mysql-community-client mysql-shell mysql-router-community percona-xtrabackup-84 mysqld-exporter",
		"kubernetes":     "kubeadm kubelet kubectl",
		"java-runtime":   "java-17-openjdk-src java-17-openjdk-headless",
		"kube-runtime":   "containerd.io",
		"agensgraph":     "agensgraph-$v",
		"agens":          "agensgraph-$v",
		"pgedge":         "pgedge-$v",
		"orioledb":       "orioledb-$v",
		"vtraces":        "victoria-traces",
		"ansible":        ansibleComboEL9,
		"node-bootstrap": nodeBootstrapComboEL9,
	}

	for key, want := range expect {
		if got := ec.AliasMap[key]; got != want {
			t.Fatalf("unexpected alias for %s: want %q, got %q", key, want, got)
		}
	}
	if ec.AliasMap["ansible"] == ec.AliasMap["node-bootstrap"] {
		t.Fatalf("ansible should be a lean combo and must differ from node-bootstrap, got %q", ec.AliasMap["ansible"])
	}
	if _, ok := ec.AliasMap["kafka"]; ok {
		t.Fatal("legacy kafka alias should not shadow the raw kafka package")
	}
}

func TestNodeBootstrapUsesSowWithoutLegacyRepoTools(t *testing.T) {
	tests := []struct {
		name      string
		combo     string
		forbidden []string
	}{
		{name: "el7", combo: nodeBootstrapComboEL7, forbidden: []string{"createrepo_c"}},
		{name: "el8", combo: nodeBootstrapComboEL8, forbidden: []string{"createrepo_c", "modulemd-tools"}},
		{name: "el9", combo: nodeBootstrapComboEL9, forbidden: []string{"createrepo_c", "modulemd-tools"}},
		{name: "el10", combo: nodeBootstrapComboEL10, forbidden: []string{"createrepo_c"}},
		{name: "debian", combo: nodeBootstrapComboD12, forbidden: []string{"dpkg-dev"}},
		{name: "ubuntu", combo: nodeBootstrapComboU22, forbidden: []string{"dpkg-dev"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages := " " + tt.combo + " "
			if !strings.Contains(packages, " sow ") {
				t.Fatalf("node-bootstrap %q does not contain sow", tt.combo)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(packages, " "+forbidden+" ") {
					t.Fatalf("node-bootstrap %q still contains %q", tt.combo, forbidden)
				}
			}
		})
	}
}

func TestLoadAliasMapNormalizesLegacyInfraPackageNames(t *testing.T) {
	legacy := map[string]string{
		"blackbox_exporter":   "blackbox-exporter",
		"kafka_exporter":      "kafka-exporter",
		"keepalived_exporter": "keepalived-exporter",
		"mongodb_exporter":    "mongodb-exporter",
		"mysqld_exporter":     "mysqld-exporter",
		"nginx_exporter":      "nginx-exporter",
		"node_exporter":       "node-exporter",
		"pg_exporter":         "pg-exporter",
		"pg_timetable":        "pg-timetable",
		"pgbackrest_exporter": "pgbackrest-exporter",
		"redis_exporter":      "redis-exporter",
		"zfs_exporter":        "zfs-exporter",
	}
	tests := []struct {
		name      string
		osType    string
		osCode    string
		ivoryOld  string
		ivoryWant string
	}{
		{name: "el", osType: config.DistroEL, osCode: "el9", ivoryOld: "ivorysql5", ivoryWant: "ivorysql-18"},
		{name: "deb", osType: config.DistroDEB, osCode: "u24", ivoryOld: "ivorysql-5", ivoryWant: "ivorysql-18"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)
			legacyPackages := make(map[string]bool, len(legacy)+1)
			for oldName, canonical := range legacy {
				legacyPackages[oldName] = true
				if got := ec.AliasMap[oldName]; got != canonical {
					t.Fatalf("legacy alias %s = %q, want %q", oldName, got, canonical)
				}
				if got, ok := ec.AliasMap[canonical]; ok {
					t.Fatalf("canonical package %s should not have an identity alias, got %q", canonical, got)
				}
			}
			if got := ec.AliasMap[tt.ivoryOld]; got != tt.ivoryWant {
				t.Fatalf("legacy alias %s = %q, want %q", tt.ivoryOld, got, tt.ivoryWant)
			}
			legacyPackages[tt.ivoryOld] = true
			for alias, value := range ec.AliasMap {
				for _, pkg := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
					if legacyPackages[pkg] {
						t.Fatalf("alias %s still references legacy package name %s", alias, pkg)
					}
				}
			}
			for _, canonical := range []string{"ivorysql-18", "ivorysql-18-contrib"} {
				if got, ok := ec.AliasMap[canonical]; ok {
					t.Fatalf("canonical package %s should not have an identity alias, got %q", canonical, got)
				}
			}
		})
	}
}

func TestLoadAliasMapIncludesBetaStaticAliases(t *testing.T) {
	beta := PostgresBetaMajorVersion
	tests := []struct {
		name   string
		osType string
		osCode string
		want   map[string]string
	}{
		{
			name:   "el",
			osType: config.DistroEL,
			osCode: "el9",
			want: map[string]string{
				fmt.Sprintf("pg%d", beta):       fmt.Sprintf("postgresql%d postgresql%d-server postgresql%d-libs postgresql%d-contrib postgresql%d-plperl postgresql%d-plpython3 postgresql%d-pltcl", beta, beta, beta, beta, beta, beta, beta),
				fmt.Sprintf("pg%d-mini", beta):  fmt.Sprintf("postgresql%d postgresql%d-server postgresql%d-libs postgresql%d-contrib", beta, beta, beta, beta),
				fmt.Sprintf("pg%d-devel", beta): fmt.Sprintf("postgresql%d-devel", beta),
				fmt.Sprintf("pg%d-basic", beta): fmt.Sprintf("pg_repack_%d wal2json_%d pgvector_%d", beta, beta, beta),
				"pgsql-devel":                   "postgresql$v-devel",
			},
		},
		{
			name:   "deb",
			osType: config.DistroDEB,
			osCode: "u24",
			want: map[string]string{
				fmt.Sprintf("pg%d", beta):       fmt.Sprintf("postgresql-%d postgresql-client-%d postgresql-plpython3-%d postgresql-plperl-%d postgresql-pltcl-%d", beta, beta, beta, beta, beta),
				fmt.Sprintf("pg%d-mini", beta):  fmt.Sprintf("postgresql-%d postgresql-client-%d", beta, beta),
				fmt.Sprintf("pg%d-devel", beta): fmt.Sprintf("postgresql-server-dev-%d", beta),
				fmt.Sprintf("pg%d-basic", beta): fmt.Sprintf("postgresql-%d-repack postgresql-%d-wal2json postgresql-%d-pgvector", beta, beta, beta),
				"pgsql-devel":                   "postgresql-server-dev-$v",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)

			for key, want := range tt.want {
				if got := ec.AliasMap[key]; got != want {
					t.Fatalf("unexpected alias for %s: want %q, got %q", key, want, got)
				}
			}
		})
	}
}

func TestLoadAliasMapIncludesPolarDBCanonicalAlias(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
	}{
		{name: "el", osType: config.DistroEL, osCode: "el9"},
		{name: "deb", osType: config.DistroDEB, osCode: "u24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)

			for _, key := range []string{"polardb", "polar"} {
				if got := ec.AliasMap[key]; got != "polardb-$v" {
					t.Fatalf("unexpected alias for %s: want %q, got %q", key, "polardb-$v", got)
				}
			}
		})
	}
}

func TestLoadAliasMapIncludesCanonicalForkAliases(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
		want   map[string]string
	}{
		{
			name:   "el",
			osType: config.DistroEL,
			osCode: "el9",
			want: map[string]string{
				"openhalo":    "openhalodb-$v",
				"openhalodb":  "openhalodb-$v",
				"ivorysql":    "ivorysql-$v ivorysql-$v-contrib",
				"ivorysqldb":  "ivorysql-$v ivorysql-$v-contrib",
				"babelfish":   "babelfish-$v",
				"babelfishpg": "babelfish-$v",
				"cloudberry":  "cloudberry cloudberry-backup cloudberry-pxf",
			},
		},
		{
			name:   "deb",
			osType: config.DistroDEB,
			osCode: "u24",
			want: map[string]string{
				"openhalo":    "openhalodb-$v",
				"openhalodb":  "openhalodb-$v",
				"ivorysql":    "ivorysql-$v ivorysql-$v-contrib",
				"ivorysqldb":  "ivorysql-$v ivorysql-$v-contrib",
				"babelfish":   "babelfish-$v",
				"babelfishpg": "babelfish-$v",
				"cloudberry":  "cloudberry cloudberry-backup cloudberry-pxf",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)

			for key, want := range tt.want {
				if got := ec.AliasMap[key]; got != want {
					t.Fatalf("unexpected alias for %s: want %q, got %q", key, want, got)
				}
			}
		})
	}
}

func TestLoadAliasMapKeepsOnlyNonIdentityUtilityAliases(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
		omit   []string
	}{
		{
			name:   "el",
			osType: config.DistroEL,
			osCode: "el9",
			omit:   []string{"pg-hardstorage", "pgxnclient", "pgformatter", "pgcopydb", "pgloader"},
		},
		{
			name:   "deb",
			osType: config.DistroDEB,
			osCode: "u24",
			omit:   []string{"pg-hardstorage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)
			if got := ec.AliasMap["pg_hardstorage"]; got != "pg-hardstorage" {
				t.Fatalf("pg_hardstorage compatibility alias = %q, want pg-hardstorage", got)
			}

			for _, key := range tt.omit {
				if got, ok := ec.AliasMap[key]; ok {
					t.Fatalf("unexpected alias for unavailable package %s: %q", key, got)
				}
			}
		})
	}
}

func TestLoadAliasMapOmitsIdentityAliases(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
	}{
		{name: "el", osType: config.DistroEL, osCode: "el9"},
		{name: "deb", osType: config.DistroDEB, osCode: "u24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)
			for alias, packages := range ec.AliasMap {
				if alias == strings.TrimSpace(packages) {
					t.Fatalf("identity alias %s -> %s should be omitted", alias, packages)
				}
			}
		})
	}
}

func TestLoadAliasMapIncludesConfgenMiscAliases(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
		want   map[string]string
	}{
		{
			name:   "el",
			osType: config.DistroEL,
			osCode: "el9",
			want: map[string]string{
				"supabase":    "pg_tle_$v,pgvector_$v,pg_cron_$v,pgsodium_$v,pg_graphql_$v,pg_jsonschema_$v,wrappers_$v,vault_$v,pgjwt_$v,pgsql_http_$v,pg_net_$v,supautils_$v,index_advisor_$v,safeupdate_$v,pg_plan_filter_$v",
				"infra-extra": "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscli vlagent victoria-traces grafana-victorialogs-ds rclone mysqld-exporter mongodb-exporter kafka-exporter",
				"victoria":    "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscli vlagent victoria-traces grafana-victorialogs-ds",
				"vmetrics":    "victoria-metrics victoria-metrics-cluster vmutils",
				"vlogs":       "victoria-logs vlogscli vlagent",
			},
		},
		{
			name:   "deb",
			osType: config.DistroDEB,
			osCode: "u24",
			want: map[string]string{
				"postgresql":  "postgresql-$v",
				"supabase":    "postgresql-$v-pg-tle,postgresql-$v-pg-graphql,postgresql-$v-pg-jsonschema,postgresql-$v-wrappers,postgresql-$v-pgvector,postgresql-$v-cron,postgresql-$v-pgsodium,postgresql-$v-vault,postgresql-$v-pgjwt,postgresql-$v-http,postgresql-$v-pg-net,postgresql-$v-supautils,postgresql-$v-index-advisor,postgresql-$v-pg-safeupdate,postgresql-$v-pg-plan-filter",
				"infra-extra": "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscli vlagent victoria-traces grafana-victorialogs-ds rclone mysqld-exporter mongodb-exporter kafka-exporter",
				"victoria":    "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscli vlagent victoria-traces grafana-victorialogs-ds",
				"vmetrics":    "victoria-metrics victoria-metrics-cluster vmutils",
				"vlogs":       "victoria-logs vlogscli vlagent",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)
			for key, want := range tt.want {
				if got := ec.AliasMap[key]; got != want {
					t.Fatalf("unexpected alias for %s: want %q, got %q", key, want, got)
				}
			}
		})
	}
}

func TestLoadAliasMapPreservesGreenplumAlias(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroEL, "el9", "amd64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)
	if got := ec.AliasMap["greenplum"]; got != "open-source-greenplum-db-7" {
		t.Fatalf("greenplum compatibility alias = %q, want open-source-greenplum-db-7", got)
	}
}

func TestLoadAliasMapIncludesPgTDEAlias(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
		want   string
	}{
		{name: "el", osType: config.DistroEL, osCode: "el9", want: "pgtde-18,pgtde-18-contrib"},
		{name: "deb", osType: config.DistroDEB, osCode: "u24", want: "pgtde-18 pgtde-18-contrib"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)
			if got := ec.AliasMap["pgtde"]; got != tt.want {
				t.Fatalf("pgtde alias = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadAliasMapPreservesCompatibilityForkAliases(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
	}{
		{name: "el", osType: config.DistroEL, osCode: "el9"},
		{name: "deb", osType: config.DistroDEB, osCode: "u24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, "amd64")
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)
			for key, want := range map[string]string{
				"agens":       "agensgraph-$v",
				"polar":       "polardb-$v",
				"openhalo":    "openhalodb-$v",
				"ivorysqldb":  "ivorysql-$v ivorysql-$v-contrib",
				"babelfishpg": "babelfish-$v",
			} {
				if got := ec.AliasMap[key]; got != want {
					t.Fatalf("compatibility alias %s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestLoadAliasMapRequestedAliasesU22(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroDEB, "u22", "amd64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)

	expect := map[string]string{
		"docker":         "docker-ce docker-compose-plugin",
		"kafka-stack":    "kafka kafka-exporter jmx-exporter",
		"mysql":          "mysql-server mysql-client mysql-shell mysql-router-community percona-xtrabackup-84 mysqld-exporter",
		"kubernetes":     "kubeadm kubelet kubectl",
		"java-runtime":   "openjdk-17-jdk",
		"kube-runtime":   "containerd.io",
		"agensgraph":     "agensgraph-$v",
		"pgedge":         "pgedge-$v",
		"orioledb":       "orioledb-$v",
		"vtraces":        "victoria-traces",
		"ansible":        ansibleComboDEB,
		"node-bootstrap": nodeBootstrapComboU22,
	}

	for key, want := range expect {
		if got := ec.AliasMap[key]; got != want {
			t.Fatalf("unexpected alias for %s: want %q, got %q", key, want, got)
		}
	}
	if ec.AliasMap["ansible"] == ec.AliasMap["node-bootstrap"] {
		t.Fatalf("ansible should be a lean combo and must differ from node-bootstrap, got %q", ec.AliasMap["ansible"])
	}
	if _, ok := ec.AliasMap["kafka"]; ok {
		t.Fatal("legacy kafka alias should not shadow the raw kafka package")
	}
}

func TestLoadAliasMapRequestedAliasesU26(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroDEB, "u26", "amd64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)

	expect := map[string]string{
		"docker":         "docker-ce docker-compose-plugin",
		"kafka-stack":    "kafka kafka-exporter jmx-exporter",
		"mysql":          "mysql-server mysql-client mysql-shell mysql-router-community percona-xtrabackup-84 mysqld-exporter",
		"kubernetes":     "kubeadm kubelet kubectl",
		"java-runtime":   "openjdk-17-jdk",
		"kube-runtime":   "containerd.io",
		"agensgraph":     "agensgraph-$v",
		"pgedge":         "pgedge-$v",
		"orioledb":       "orioledb-$v",
		"vtraces":        "victoria-traces",
		"ansible":        ansibleComboDEB,
		"node-bootstrap": nodeBootstrapComboU22,
	}

	for key, want := range expect {
		if got := ec.AliasMap[key]; got != want {
			t.Fatalf("unexpected alias for %s: want %q, got %q", key, want, got)
		}
	}
	if ec.AliasMap["ansible"] == ec.AliasMap["node-bootstrap"] {
		t.Fatalf("ansible should be a lean combo and must differ from node-bootstrap, got %q", ec.AliasMap["ansible"])
	}
	if _, ok := ec.AliasMap["kafka"]; ok {
		t.Fatal("legacy kafka alias should not shadow the raw kafka package")
	}
}

func TestLoadAliasMapOSOverrides(t *testing.T) {
	tests := []struct {
		name   string
		osType string
		osCode string
		osArch string
		key    string
		want   string
	}{
		{
			name:   "el8_ansible_combo",
			osType: config.DistroEL,
			osCode: "el8",
			osArch: "amd64",
			key:    "ansible",
			want:   ansibleComboEL8,
		},
		{
			name:   "el10_java_runtime",
			osType: config.DistroEL,
			osCode: "el10",
			osArch: "amd64",
			key:    "java-runtime",
			want:   "java-21-openjdk-src java-21-openjdk-headless",
		},
		{
			name:   "el10_ansible_combo",
			osType: config.DistroEL,
			osCode: "el10",
			osArch: "amd64",
			key:    "ansible",
			want:   ansibleComboEL10,
		},
		{
			name:   "el10_node_bootstrap",
			osType: config.DistroEL,
			osCode: "el10",
			osArch: "amd64",
			key:    "node-bootstrap",
			want:   nodeBootstrapComboEL10,
		},
		{
			name:   "d13_java_runtime",
			osType: config.DistroDEB,
			osCode: "d13",
			osArch: "amd64",
			key:    "java-runtime",
			want:   "openjdk-21-jdk",
		},
		{
			name:   "d13_ansible_combo",
			osType: config.DistroDEB,
			osCode: "d13",
			osArch: "amd64",
			key:    "ansible",
			want:   ansibleComboDEB,
		},
		{
			name:   "d11_node_bootstrap",
			osType: config.DistroDEB,
			osCode: "d11",
			osArch: "amd64",
			key:    "node-bootstrap",
			want:   nodeBootstrapComboD12,
		},
		{
			name:   "el7_kube_runtime",
			osType: config.DistroEL,
			osCode: "el7",
			osArch: "amd64",
			key:    "kube-runtime",
			want:   "containerd.io cri-dockerd",
		},
		{
			name:   "el7_oriole_disabled",
			osType: config.DistroEL,
			osCode: "el7",
			osArch: "amd64",
			key:    "oriole",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withAliasMapTestEnv(t, tt.osType, tt.osCode, tt.osArch)
			defer cleanup()

			ec := &ExtensionCatalog{}
			ec.LoadAliasMap(config.OSType)

			if got := ec.AliasMap[tt.key]; got != tt.want {
				t.Fatalf("unexpected alias for %s: want %q, got %q", tt.key, tt.want, got)
			}
		})
	}
}

func TestLoadAliasMapEl9Arm64UsesNoarchPatroniAliases(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroEL, "el9", "arm64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)

	if got := ec.AliasMap["patroni"]; got != "patroni.noarch patroni-etcd.noarch" {
		t.Fatalf("unexpected patroni alias for el9 arm64: %q", got)
	}
	if got := ec.AliasMap["pgsql-common"]; got != "patroni.noarch patroni-etcd.noarch pgbouncer pgbackrest pg-exporter pgbackrest-exporter vip-manager" {
		t.Fatalf("unexpected pgsql-common alias for el9 arm64: %q", got)
	}
}
