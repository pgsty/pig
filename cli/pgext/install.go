package pgext

import (
	"fmt"
	"pig/cli/pgsql"
	"pig/cli/utils"
	"pig/internal/config"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

var CategoryList = []string{"TIME", "GIS", "RAG", "FTS", "OLAP", "FEAT", "LANG", "TYPE", "FUNC", "ADMIN", "STAT", "SEC", "FDW", "SIM", "ETL"}

var PostgresPackageMap map[string]string

var PostgresVersions = []string{"17", "16", "15", "14", "13"}

func BuildPostgresPackageMap(distro string) map[string]string {
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
		return pkgMap
	}
	return map[string]string{}
}

// InstallExtensions installs extensions based on provided names, aliases, or categories
func InstallExtensions(names []string, pg *pgsql.PostgresInstallation) error {
	var installCmds []string
	if config.OSType == config.DistroEL {
		installCmds = append(installCmds, []string{"yum", "install", "-y"}...)
	} else if config.OSType == config.DistroDEB {
		installCmds = append(installCmds, []string{"apt-get", "install", "-y"}...)
	} else {
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	if err := InitExtension(nil); err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}

	PostgresPackageMap = BuildPostgresPackageMap(config.OSType)
	var pkgNames []string
	pkgNameSet := make(map[string]struct{})
	for _, name := range names {
		ext, ok := ExtNameMap[name]
		if !ok {
			ext, ok = ExtAliasMap[name]
		}
		if !ok {
			// try to find in PostgresPackageMap (if it is not a postgres extension)
			if pgPkg, ok := PostgresPackageMap[name]; ok {
				// splite result and replace $v with pg.MajorVersion
				parts := strings.Split(strings.Replace(pgPkg, ",", " ", -1), " ")
				// iterate and replace $v
				for _, part := range parts {
					partStr := strings.ReplaceAll(part, "$v", strconv.Itoa(pg.MajorVersion))
					if _, exists := pkgNameSet[partStr]; !exists {
						pkgNames = append(pkgNames, partStr)
						pkgNameSet[partStr] = struct{}{}
					}
				}
				continue
			} else {
				logrus.Debugf("can not found '%s' in extension name or alias", name)
				continue
			}
		}
		pkgName := ext.PackageName(pg.MajorVersion)
		if pkgName == "" {
			logrus.Warnf("no package found for extension %s", ext.Name)
			continue
		}
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
		if _, exists := pkgNameSet[pkgName]; !exists {
			pkgNames = append(pkgNames, pkgName)
			pkgNameSet[pkgName] = struct{}{}
		}
	}

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be installed")
	}
	installCmds = append(installCmds, pkgNames...)
	logrus.Infof("installing extensions: %s", strings.Join(installCmds, " "))

	return utils.SudoCommand(installCmds)
}

// RemoveExtension
func RemoveExtensions(names []string, pg *pgsql.PostgresInstallation) error {
	var removeCmds []string
	if config.OSType == config.DistroEL {
		removeCmds = append(removeCmds, []string{"yum", "remove", "-y"}...)
	} else if config.OSType == config.DistroDEB {
		removeCmds = append(removeCmds, []string{"apt-get", "remove", "-y"}...)
	} else {
		return fmt.Errorf("unsupported OS type: %s", config.OSType)
	}

	if err := InitExtension(nil); err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no extension names provided")
	}

	var pkgNames []string
	pkgNameSet := make(map[string]struct{})
	for _, name := range names {
		ext, ok := ExtNameMap[name]
		if !ok {
			ext, ok = ExtAliasMap[name]
		}
		if !ok {
			logrus.Warnf("can not found '%s' in extension name or alias", name)
			continue
		}
		pkgName := ext.PackageName(pg.MajorVersion)
		if pkgName == "" {
			logrus.Warnf("no package found for extension %s", ext.Name)
			continue
		}
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
		if _, exists := pkgNameSet[pkgName]; !exists {
			pkgNames = append(pkgNames, pkgName)
			pkgNameSet[pkgName] = struct{}{}
		}
	}

	if len(pkgNames) == 0 {
		return fmt.Errorf("no packages to be removed")
	}
	removeCmds = append(removeCmds, pkgNames...)
	logrus.Infof("removing extensions: %s", strings.Join(removeCmds, " "))

	return utils.SudoCommand(removeCmds)

}
