package pgext

import (
	"fmt"
	"pig/internal/config"
	"strings"
)

func InitPackageMap(distro string) map[string]string {
	if distro == "" {
		distro = config.OSType
	}
	switch distro {
	case "el", "rpm", "el8", "el9":
		pkgMap := map[string]string{
			"postgresql":   "postgresql$v*",
			"pgsql-common": "patroni patroni-etcd pgbouncer pgbackrest pg_exporter pgbadger vip-manager",
			"patroni":      "patroni patroni-etcd",
			"pgbouncer":    "pgbouncer",
			"pgbackrest":   "pgbackrest",
			"pg_exporter":  "pg_exporter",
			"vip-manager":  "vip-manager",
			"pgbadger":     "pgbadger",
			"pg_activity":  "pg_activity",
			"pg_filedump":  "pg_filedump",
			"pgxnclient":   "pgxnclient",
			"pgformatter":  "pgformatter",
			"pgcopydb":     "pgcopydb",
			"pgloader":     "pgloader",
			"pg_timetable": "pg_timetable",
			"wiltondb":     "wiltondb",
			"polardb":      "PolarDB",
			"ivorysql":     "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test",
			"ivorysql-all": "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test ivorysql3-docs ivorysql3-devel ivorysql3-llvmjit",
		}
		pkgMapTmpl := map[string]string{
			"pgsql":        "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit",
			"pgsql-core":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-test postgresql$v-devel postgresql$v-llvmjit",
			"pgsql-simple": "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl",
			"pgsql-client": "postgresql$v",
			"pgsql-server": "postgresql$v-server postgresql$v-libs postgresql$v-contrib",
			"pgsql-devel":  "postgresql$v-devel",
			"pgsql-basic":  "pg_repack_$v* wal2json_$v* pgvector_$v*",
		}
		for k, v := range pkgMapTmpl {
			pkgMap[k] = v
		}
		for _, ver := range PostgresVersions {
			for k, v := range pkgMapTmpl {
				key := strings.Replace(k, "pgsql", fmt.Sprintf("pg%s", ver), 1)
				value := strings.Replace(v, "$v", ver, -1)
				pkgMap[key] = value
			}
		}
		PostgresPackageMap = pkgMap
		return pkgMap
	case "deb", "d10", "d11", "d12", "u20", "u22", "u24":
		pkgMap := map[string]string{
			"postgresql":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v",
			"pgsql-common": "patroni pgbouncer pgbackrest pg-exporter pgbadger vip-manager",
			"patroni":      "patroni",
			"pgbouncer":    "pgbouncer",
			"pgbackrest":   "pgbackrest",
			"pg_exporter":  "pg-exporter",
			"vip-manager":  "vip-manager",
			"pgbadger":     "pgbadger",
			"pg_activity":  "pg-activity",
			"pg_filedump":  "postgresql-filedump",
			"pgxnclient":   "pgxnclient",
			"pgformatter":  "pgformatter",
			"pgcopydb":     "pgcopydb",
			"pgloader":     "pgloader",
			"pg_timetable": "pg-timetable",
			"wiltondb":     "wiltondb",
			"polardb":      "polardb-for-postgresql",
		}
		pkgMapTmpl := map[string]string{
			"pgsql":        "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v",
			"pgsql-main":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v postgresql-$v-repack postgresql-$v-wal2json postgresql-$v-pgvector",
			"pgsql-core":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v",
			"pgsql-simple": "postgresql-$v postgresql-client-$v postgresql-plperl-$v postgresql-plpython3-$v postgresql-pltcl-$v",
			"pgsql-client": "postgresql-client-$v",
			"pgsql-server": "postgresql-$v",
			"pgsql-devel":  "postgresql-server-dev-$v",
			"pgsql-basic":  "postgresql-$v-repack postgresql-$v-wal2json postgresql-$v-pgvector",
		}
		for k, v := range pkgMapTmpl {
			pkgMap[k] = v
		}
		for _, ver := range PostgresVersions {
			for k, v := range pkgMapTmpl {
				key := strings.Replace(k, "pgsql", fmt.Sprintf("pg%s", ver), 1)
				value := strings.Replace(v, "$v", ver, -1)
				pkgMap[key] = value
			}
		}
		PostgresPackageMap = pkgMap
		return pkgMap
	}
	PostgresPackageMap = map[string]string{}
	return map[string]string{}
}
