package ext

import (
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
		"orioledb":       "orioledb_17 oriolepg_17",
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
		"orioledb":       "oriolepg-17-orioledb oriolepg-17",
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

func TestLoadAliasMapArchOverridesStillApply(t *testing.T) {
	cleanup := withAliasMapTestEnv(t, config.DistroEL, "el9", "arm64")
	defer cleanup()

	ec := &ExtensionCatalog{}
	ec.LoadAliasMap(config.OSType)

	if got := ec.AliasMap["patroni"]; got != "patroni-4.1.0 patroni-etcd-4.1.0" {
		t.Fatalf("unexpected patroni alias after arch override: %q", got)
	}
	if got := ec.AliasMap["pgsql-common"]; got != "patroni-4.1.0 patroni-etcd-4.1.0 pgbouncer pgbackrest pg_exporter pgbackrest_exporter vip-manager" {
		t.Fatalf("unexpected pgsql-common alias after arch override: %q", got)
	}
}
