package ext

import (
	"fmt"
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
		"kafka":          "kafka kafka_exporter",
		"kubernetes":     "kubeadm kubelet kubectl",
		"java-runtime":   "java-17-openjdk-src java-17-openjdk-headless",
		"kube-runtime":   "containerd.io",
		"agensgraph":     "agensgraph_$v",
		"agens":          "agensgraph_$v",
		"pgedge":         "pgedge_$v spock_$v lolor_$v snowflake_$v",
		"orioledb":       "orioledb-$v",
		"vtraces":        "victoria-traces",
		"hunspell":       "hunspell_cs_cz_$v hunspell_de_de_$v hunspell_en_us_$v hunspell_fr_$v hunspell_ne_np_$v hunspell_nl_nl_$v hunspell_nn_no_$v hunspell_ru_ru_$v hunspell_ru_ru_aot_$v",
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
				if got := ec.AliasMap[key]; got != "polardb-17" {
					t.Fatalf("unexpected alias for %s: want %q, got %q", key, "polardb-17", got)
				}
			}
		})
	}
}

func TestLoadAliasMapIvorySQLUsesPG18Package(t *testing.T) {
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

			if got := ec.AliasMap["ivorysqldb"]; got != "ivorysql-18" {
				t.Fatalf("unexpected ivorysqldb alias: want %q, got %q", "ivorysql-18", got)
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
		"kafka":          "kafka kafka-exporter",
		"kubernetes":     "kubeadm kubelet kubectl",
		"java-runtime":   "openjdk-17-jdk",
		"kube-runtime":   "containerd.io",
		"agensgraph":     "agensgraph-$v",
		"agens":          "agensgraph-$v",
		"pgedge":         "pgedge-$v pgedge-$v-spock pgedge-$v-lolor pgedge-$v-snowflake",
		"orioledb":       "orioledb-$v",
		"vtraces":        "victoria-traces",
		"hunspell":       "postgresql-$v-hunspell-cs-cz,postgresql-$v-hunspell-de-de,postgresql-$v-hunspell-en-us,postgresql-$v-hunspell-fr,postgresql-$v-hunspell-ne-np,postgresql-$v-hunspell-nl-nl,postgresql-$v-hunspell-nn-no,postgresql-$v-hunspell-ru-ru,postgresql-$v-hunspell-ru-ru-aot",
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
}

func TestLoadAliasMapRequestedAliasesU26(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroDEB, "u26", "amd64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)

	expect := map[string]string{
		"docker":         "docker-ce docker-compose-plugin",
		"kafka":          "kafka kafka-exporter",
		"kubernetes":     "kubeadm kubelet kubectl",
		"java-runtime":   "openjdk-17-jdk",
		"kube-runtime":   "containerd.io",
		"agensgraph":     "agensgraph-$v",
		"agens":          "agensgraph-$v",
		"pgedge":         "pgedge-$v pgedge-$v-spock pgedge-$v-lolor pgedge-$v-snowflake",
		"orioledb":       "orioledb-$v",
		"vtraces":        "victoria-traces",
		"hunspell":       "postgresql-$v-hunspell-cs-cz,postgresql-$v-hunspell-de-de,postgresql-$v-hunspell-en-us,postgresql-$v-hunspell-fr,postgresql-$v-hunspell-ne-np,postgresql-$v-hunspell-nl-nl,postgresql-$v-hunspell-nn-no,postgresql-$v-hunspell-ru-ru,postgresql-$v-hunspell-ru-ru-aot",
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
	if got := ec.AliasMap["pgsql-common"]; got != "patroni.noarch patroni-etcd.noarch pgbouncer pgbackrest pg_exporter pgbackrest_exporter vip-manager" {
		t.Fatalf("unexpected pgsql-common alias for el9 arm64: %q", got)
	}
}
