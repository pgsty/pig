package ext

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"slices"
	"sort"
	"strings"

	_ "embed"

	"github.com/sirupsen/logrus"
)

//go:embed assets/extension.csv
var embedExtensionData []byte

// The global default extension catalog (use config file if applicable, fallback to embedded data)
var Catalog, _ = NewExtensionCatalog()

// ExtensionCatalog hold extension metadata, for given DataPath or embed data
type ExtensionCatalog struct {
	Extensions  []*Extension
	ExtNameMap  map[string]*Extension
	ExtPkgMap   map[string]*Extension
	Dependency  map[string][]string
	ControlLess map[string]bool
	DataPath    string
	AliasMap    map[string]string
}

// ReloadCatalog reloads the extension catalog from the default data path
func ReloadCatalog(paths ...string) (err error) {
	Catalog, err = NewExtensionCatalog(paths...)
	return
}

// NewExtensionCatalog creates a new ExtensionCatalog, using embedded data if any error occurs
func NewExtensionCatalog(paths ...string) (*ExtensionCatalog, error) {
	ec := &ExtensionCatalog{DataPath: "embedded"}
	var data []byte
	var defaultCsvPath string
	if config.ConfigDir != "" {
		defaultCsvPath = filepath.Join(config.ConfigDir, "extension.csv")
		if !slices.Contains(paths, defaultCsvPath) {
			paths = append(paths, defaultCsvPath)
		}
	}

	for _, path := range paths {
		if fileData, err := os.ReadFile(path); err == nil {
			logrus.Debugf("check extension csv data file: %s", path)
			data = fileData
			ec.DataPath = path
			break
		}
	}
	if err := ec.Load(data); err != nil {
		if ec.DataPath != defaultCsvPath {
			logrus.Debugf("failed to load extension data from %s, using embedded", ec.DataPath)
		} else {
			logrus.Debugf("extension data not found at default path, using embedded")
		}
		ec.DataPath = "embedded"
		if err = ec.Load(embedExtensionData); err != nil {
			logrus.Errorf("failed to parse embedded extension data: %v", err)
			return nil, fmt.Errorf("failed to load extension catalog: %w", err)
		}
		return ec, nil
	}
	logrus.Debugf("extension catalog loaded: %s", ec.DataPath)
	return ec, nil
}

// Load loads extension data from the provided data or embedded data
func (ec *ExtensionCatalog) Load(data []byte) error {
	var csvReader *csv.Reader
	if data == nil {
		data = embedExtensionData
		ec.DataPath = "embedded"
	}
	csvReader = csv.NewReader(bytes.NewReader(data))
	if _, err := csvReader.Read(); err != nil {
		return fmt.Errorf("failed to read CSV header: %v", err)
	}

	// read & parse all records
	records, err := csvReader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV records: %v", err)
	}
	extensions := make([]Extension, 0, len(records))
	for _, record := range records {
		ext, err := ParseExtension(record)
		if err != nil {
			logrus.Debugf("failed to parse extension record: %v", err)
			return fmt.Errorf("failed to parse extension record: %v", err)
		}
		extensions = append(extensions, *ext)
	}
	sort.Slice(extensions, func(i, j int) bool {
		return extensions[i].ID < extensions[j].ID
	})

	// update peripheral data
	ec.Extensions = make([]*Extension, len(extensions))
	ec.ExtNameMap = make(map[string]*Extension)
	ec.ExtPkgMap = make(map[string]*Extension)
	ec.Dependency = make(map[string][]string)
	for i := range extensions {
		ext := &extensions[i]
		ec.Extensions[i] = ext
		ec.ExtNameMap[ext.Name] = ext
		// Use Pkg field as alias for lead extensions
		if ext.Pkg != "" && ext.Lead {
			ec.ExtPkgMap[ext.Pkg] = ext
		}
		if len(ext.Requires) > 0 {
			for _, req := range ext.Requires {
				if _, exists := ec.Dependency[req]; !exists {
					ec.Dependency[req] = []string{ext.Name}
				} else {
					ec.Dependency[req] = append(ec.Dependency[req], ext.Name)
				}
			}
		}
	}

	var ctrlLess = make(map[string]bool)
	for _, ext := range ec.Extensions {
		if ext.HasLib && !ext.NeedDDL {
			ctrlLess[ext.Name] = true
		}
	}
	ec.ControlLess = ctrlLess
	return nil
}

const (
	ansibleComboEL7  = "ansible sshpass"
	ansibleComboEL8  = "ansible sshpass python3.12-jmespath python3-cryptography"
	ansibleComboEL9  = "ansible sshpass python3-jmespath"
	ansibleComboEL10 = "ansible sshpass"
	ansibleComboDEB  = "ansible sshpass python3-jmespath"

	nodeBootstrapComboEL7  = "ansible python3 python36-requests python36-idna yum-utils createrepo_c sshpass"
	nodeBootstrapComboEL8  = "ansible python3 python3-requests python3.12-jmespath python3-cryptography dnf-utils modulemd-tools createrepo_c sshpass"
	nodeBootstrapComboEL9  = "ansible python3 python3-requests python3-jmespath python3-cryptography dnf-utils modulemd-tools createrepo_c sshpass"
	nodeBootstrapComboEL10 = "ansible python3 python3-requests python3-jmespath python3-cryptography dnf-utils createrepo_c sshpass crypto-policies-scripts"
	nodeBootstrapComboD12  = "ansible python3 python3-requests python3-jmespath dpkg-dev sshpass tnftp linux-perf"
	nodeBootstrapComboU22  = "ansible python3 python3-requests python3-jmespath dpkg-dev sshpass ftp linux-tools-generic"
)

// OSAliasOverride contains package name overrides for specific OS code combinations
// Key format: "el9", value is a map of alias -> package list
var OSAliasOverride = map[string]map[string]string{
	"el7": {
		"java-runtime":   "java-11-openjdk-src java-11-openjdk-headless",
		"ansible":        ansibleComboEL7,
		"node-bootstrap": nodeBootstrapComboEL7,
		"kube-runtime":   "containerd.io cri-dockerd",
		"oriole":         "",
	},
	"el8": {
		"java-runtime":   "java-17-openjdk-src java-17-openjdk-headless",
		"ansible":        ansibleComboEL8,
		"node-bootstrap": nodeBootstrapComboEL8,
	},
	"el9": {
		"java-runtime":   "java-17-openjdk-src java-17-openjdk-headless",
		"ansible":        ansibleComboEL9,
		"node-bootstrap": nodeBootstrapComboEL9,
	},
	"el10": {
		"java-runtime":   "java-21-openjdk-src java-21-openjdk-headless",
		"ansible":        ansibleComboEL10,
		"node-bootstrap": nodeBootstrapComboEL10,
	},
	"d12": {
		"java-runtime":   "openjdk-17-jdk",
		"ansible":        ansibleComboDEB,
		"node-bootstrap": nodeBootstrapComboD12,
	},
	"d13": {
		"java-runtime":   "openjdk-21-jdk",
		"ansible":        ansibleComboDEB,
		"node-bootstrap": nodeBootstrapComboD12,
	},
	"u22": {
		"java-runtime":   "openjdk-17-jdk",
		"ansible":        ansibleComboDEB,
		"node-bootstrap": nodeBootstrapComboU22,
	},
	"u24": {
		"java-runtime":   "openjdk-17-jdk",
		"ansible":        ansibleComboDEB,
		"node-bootstrap": nodeBootstrapComboU22,
	},
}

// ArchAliasOverride contains package name overrides for specific OS+arch combinations
// Key format: "el9.arm64", value is a map of alias -> package names
var ArchAliasOverride = map[string]map[string]string{
	"el9.arm64": {
		"patroni":      "patroni-4.1.0 patroni-etcd-4.1.0",
		"pgsql-common": "patroni-4.1.0 patroni-etcd-4.1.0 pgbouncer pgbackrest pg_exporter pgbackrest_exporter vip-manager",
	},
}

// LoadAliasMap loads the alias map for the given distribution code
func (ec *ExtensionCatalog) LoadAliasMap(distroCode string) {
	if distroCode == "" {
		distroCode = config.OSType
	}
	ec.AliasMap = map[string]string{}
	switch distroCode {
	case "el", "rpm", "el7", "el8", "el9", "el10":
		pkgMap := map[string]string{
			"postgresql":          "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl",
			"pgsql-common":        "patroni patroni-etcd pgbouncer pgbackrest pg_exporter pgbackrest_exporter vip-manager",
			"patroni":             "patroni patroni-etcd",
			"ansible":             ansibleComboEL9,
			"node-bootstrap":      nodeBootstrapComboEL9,
			"java-runtime":        "java-17-openjdk-src java-17-openjdk-headless",
			"pgbouncer":           "pgbouncer",
			"pgbackrest":          "pgbackrest",
			"pg_exporter":         "pg_exporter",
			"pgbackrest_exporter": "pgbackrest_exporter",
			"vip-manager":         "vip-manager",
			"pgbadger":            "pgbadger",
			"pg_activity":         "pg_activity",
			"pg_filedump":         "pg_filedump",
			"pgformatter":         "pgformatter",
			"pgloader":            "pgloader",
			"pg_timetable":        "pg_timetable",
			"timescaledb-utils":   "timescaledb-tools timescaledb-event-streamer",
			"ivorysql":            "ivorysql5",
			"agensgraph":          "agensgraph_$v",
			"agens":               "agensgraph_$v",
			"polardb":             "PolarDB",
			"oriole":              "orioledb_17 oriolepg_17",
			"orioledb":            "orioledb_17 oriolepg_17",
			"pgedge":              "pgedge_$v spock_$v lolor_$v snowflake_$v",
			"babelfish":           "babelfishpg_17 babelfish_17",
			"wiltondb":            "wiltondb",
			"openhalodb":          "openhalodb_14",
			"percona-core":        "percona-postgresql18,percona-postgresql18-server,percona-postgresql18-contrib,percona-postgresql18-plperl,percona-postgresql18-plpython3,percona-postgresql18-pltcl,percona-pg_tde18",
			"percona-main":        "percona-postgresql18,percona-postgresql18-server,percona-postgresql18-contrib,percona-postgresql18-plperl,percona-postgresql18-plpython3,percona-postgresql18-pltcl,percona-pg_tde18,percona-postgis35_18,percona-postgis35_18-client,percona-postgis35_18-utils,percona-pgvector_18,percona-wal2json18,percona-pg_repack18,percona-pgaudit18,percona-pgaudit18_set_user,percona-pg_stat_monitor18,percona-pg_gather",
			"ferretdb":            "ferretdb2",
			"cloudberry":          "cloudberry",
			"duckdb":              "duckdb",
			"docker":              "docker-ce docker-compose-plugin",
			"kafka":               "kafka kafka_exporter",
			"kube-runtime":        "containerd.io",
			"kubernetes":          "kubeadm kubelet kubectl",
			"hunspell":            "hunspell_cs_cz_$v hunspell_de_de_$v hunspell_en_us_$v hunspell_fr_$v hunspell_ne_np_$v hunspell_nl_nl_$v hunspell_nn_no_$v hunspell_ru_ru_$v hunspell_ru_ru_aot_$v",
			"node":                "audit bash bind-utils bzip2 ca-certificates chkconfig chrony cronie etcd git grubby haproxy htop iotop ipvsadm jq keepalived lsof lz4 make ncdu net-tools node_exporter numactl nvme-cli openssh-clients openssh-server openssl patch pig pv python3 readline rsync socat sysstat tcpdump telnet tuned unzip uv vector vim-minimal wget yum zlib",
			"infra":               "alertmanager ansible blackbox_exporter certbot dnsmasq etcd grafana grafana-plugins grafana-victorialogs-ds grafana-victoriametrics-ds mcli nginx nginx_exporter node_exporter pev2 pg_exporter python3-certbot-nginx python3-requests redis restic victoria-logs victoria-metrics victoria-traces vlogscli vmutils",
			"vray":                "vray",
			"genai-toolbox":       "genai-toolbox",
			"clickhouse":          "clickhouse-server clickhouse-client clickhouse-common-static",
			"victoria":            "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscli vlagent grafana-victorialogs-ds",
			"vmetrics":            "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds",
			"vlogs":               "victoria-logs vlogscli vlagent grafana-victorialogs-ds",
			"vtraces":             "victoria-traces",
		}
		pkgMapTmpl := map[string]string{
			"pgsql":        "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl", // postgresql$v-llvmjit
			"pgsql-mini":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib",
			"pgsql-core":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl", // postgresql$v-llvmjit
			"pgsql-full":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit postgresql$v-test postgresql$v-devel",
			"pgsql-main":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl pg_repack_$v wal2json_$v pgvector_$v", // postgresql$v-llvmjit
			"pgsql-client": "postgresql$v",
			"pgsql-server": "postgresql$v-server postgresql$v-libs postgresql$v-contrib",
			"pgsql-devel":  "postgresql$v-devel",
			"pgsql-basic":  "pg_repack_$v wal2json_$v pgvector_$v",
		}
		for k, v := range pkgMapTmpl {
			pkgMap[k] = v
		}
		for _, ver := range PostgresActiveMajorVersions {
			for k, v := range pkgMapTmpl {
				key := strings.Replace(k, "pgsql", fmt.Sprintf("pg%d", ver), 1)
				value := strings.Replace(v, "$v", fmt.Sprintf("%d", ver), -1)
				pkgMap[key] = value
			}
		}
		ec.AliasMap = pkgMap
	case "deb", "d10", "d11", "d12", "d13", "u20", "u22", "u24":
		pkgMap := map[string]string{
			"postgresql":          "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v",
			"pgsql-common":        "patroni python3-etcd pgbouncer pgbackrest pg-exporter pgbackrest-exporter vip-manager",
			"patroni":             "patroni python3-etcd",
			"ansible":             ansibleComboDEB,
			"node-bootstrap":      nodeBootstrapComboU22,
			"java-runtime":        "openjdk-17-jdk",
			"pgbouncer":           "pgbouncer",
			"pgbackrest":          "pgbackrest",
			"pg_exporter":         "pg-exporter",
			"pgbackrest_exporter": "pgbackrest-exporter",
			"vip-manager":         "vip-manager",
			"pgbadger":            "pgbadger",
			"pg_activity":         "pg-activity",
			"pg_filedump":         "postgresql-filedump",
			"pgformatter":         "pgformatter",
			"pgloader":            "pgloader",
			"pg_timetable":        "pg-timetable",
			"timescaledb-utils":   "timescaledb-tools timescaledb-event-streamer",
			"ivorysql":            "ivorysql-5",
			"agensgraph":          "agensgraph-$v",
			"agens":               "agensgraph-$v",
			"wiltondb":            "wiltondb",
			"polardb":             "polardb-for-postgresql",
			"oriole":              "oriolepg-17 oriolepg-17-orioledb",
			"orioledb":            "oriolepg-17-orioledb oriolepg-17",
			"pgedge":              "pgedge-$v pgedge-$v-spock pgedge-$v-lolor pgedge-$v-snowflake",
			"babelfish":           "babelfishpg-17 babelfishpg-17-babelfish",
			"openhalodb":          "openhalodb-14",
			"percona-core":        "percona-postgresql-18 percona-postgresql-client-18 percona-postgresql-plperl-18 percona-postgresql-plpython3-18 percona-postgresql-pltcl-18 percona-pg-tde18",
			"percona-main":        "percona-postgresql-18 percona-postgresql-client-18 percona-postgresql-plperl-18 percona-postgresql-plpython3-18 percona-postgresql-pltcl-18 percona-pg-tde18 percona-postgresql-18-postgis-3 percona-postgresql-18-pgvector percona-postgresql-18-wal2json percona-postgresql-18-repack percona-postgresql-18-pgaudit percona-pgaudit18-set-user percona-pg-stat-monitor18 percona-pg-gather",
			"cloudberry":          "cloudberry",
			"ferretdb":            "ferretdb2",
			"duckdb":              "duckdb",
			"docker":              "docker-ce docker-compose-plugin",
			"kafka":               "kafka kafka-exporter",
			"kube-runtime":        "containerd.io",
			"kubernetes":          "kubeadm kubelet kubectl",
			"hunspell":            "postgresql-$v-hunspell-cs-cz,postgresql-$v-hunspell-de-de,postgresql-$v-hunspell-en-us,postgresql-$v-hunspell-fr,postgresql-$v-hunspell-ne-np,postgresql-$v-hunspell-nl-nl,postgresql-$v-hunspell-nn-no,postgresql-$v-hunspell-ru-ru,postgresql-$v-hunspell-ru-ru-aot",
			"node":                "acl bash bzip2 ca-certificates chrony cron etcd git haproxy htop iotop ipvsadm jq keepalived libreadline-dev lsof lz4 make ncdu net-tools node-exporter numactl nvme-cli openssh-client openssh-server openssl patch pig pv python3 rsync socat sysstat telnet tuned unzip uv vector vim-tiny wget zlib1g",
			"infra":               "alertmanager ansible blackbox-exporter certbot dnsmasq etcd grafana grafana-plugins grafana-victorialogs-ds grafana-victoriametrics-ds mcli nginx nginx-exporter node-exporter pev2 pg-exporter python3-certbot-nginx python3-requests redis restic victoria-logs victoria-metrics victoria-traces vlogscli vmutils",
			"vray":                "vray",
			"genai-toolbox":       "genai-toolbox",
			"clickhouse":          "clickhouse-server clickhouse-client clickhouse-common-static",
			"victoria":            "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscli vlagent grafana-victorialogs-ds",
			"vmetrics":            "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds",
			"vlogs":               "victoria-logs vlogscli vlagent grafana-victorialogs-ds",
			"vtraces":             "victoria-traces",
		}
		pkgMapTmpl := map[string]string{
			"pgsql":        "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v",
			"pgsql-mini":   "postgresql-$v postgresql-client-$v",
			"pgsql-core":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v",
			"pgsql-full":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v",
			"pgsql-main":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-$v-repack postgresql-$v-wal2json postgresql-$v-pgvector",
			"pgsql-client": "postgresql-client-$v",
			"pgsql-server": "postgresql-$v",
			"pgsql-devel":  "postgresql-server-dev-$v",
			"pgsql-basic":  "postgresql-$v-repack postgresql-$v-wal2json postgresql-$v-pgvector",
		}
		for k, v := range pkgMapTmpl {
			pkgMap[k] = v
		}
		for _, ver := range PostgresActiveMajorVersions {
			for k, v := range pkgMapTmpl {
				key := strings.Replace(k, "pgsql", fmt.Sprintf("pg%d", ver), 1)
				value := strings.Replace(v, "$v", fmt.Sprintf("%d", ver), -1)
				pkgMap[key] = value
			}
		}
		ec.AliasMap = pkgMap
	}

	// Apply OS code-specific alias overrides for node-level package combinations.
	if overrides, ok := OSAliasOverride[config.OSCode]; ok {
		logrus.Debugf("applying OS alias overrides for %s", config.OSCode)
		for k, v := range overrides {
			ec.AliasMap[k] = v
		}
	}

	// Apply architecture-specific overrides
	archCode := config.OSCode + "." + config.OSArch
	if overrides, ok := ArchAliasOverride[archCode]; ok {
		logrus.Debugf("applying alias overrides for %s", archCode)
		for k, v := range overrides {
			ec.AliasMap[k] = v
		}
	}
}
