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

//go:embed assets/pigsty.csv
var embedExtensionData []byte

// The global default extension catalog (use config file if applicable, fallback to embedded data)
var Catalog, _ = NewExtensionCatalog()

// ExtensionCatalog hold extension metadata, for given DataPath or embed data
type ExtensionCatalog struct {
	Extensions  []*Extension
	ExtNameMap  map[string]*Extension
	ExtAliasMap map[string]*Extension
	Dependency  map[string][]string
	ControlLess map[string]bool
	DataPath    string
	AliasMap    map[string]string
}

// NewExtensionCatalog creates a new ExtensionCatalog, using embedded data if any error occurs
func NewExtensionCatalog(paths ...string) (*ExtensionCatalog, error) {
	ec := &ExtensionCatalog{DataPath: "embedded"}
	var data []byte
	var defaultCsvPath string
	if config.ConfigDir != "" {
		defaultCsvPath = filepath.Join(config.ConfigDir, "pigsty.csv")
		if !slices.Contains(paths, defaultCsvPath) {
			paths = append(paths, defaultCsvPath)
		}
	}
	for _, path := range paths {
		if fileData, err := os.ReadFile(path); err == nil {
			data = fileData
			ec.DataPath = path
			break
		}
	}
	if err := ec.Load(data); err != nil {
		if ec.DataPath != defaultCsvPath {
			logrus.Debugf("failed to load extension data from %s: %v, fallback to embedded data", ec.DataPath, err)
		} else {
			logrus.Debugf("failed to load extension data from default path: %s, fallback to embedded data", defaultCsvPath)
		}
		ec.DataPath = "embedded"
		err = ec.Load(embedExtensionData)
		if err != nil {
			logrus.Debugf("not likely to happen: failed on parsing embedded data: %v", err)
		}
		return ec, nil

	} else {
		logrus.Debugf("load extension data from %s", ec.DataPath)
		return ec, nil
	}
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
	ec.ExtAliasMap = make(map[string]*Extension)
	ec.Dependency = make(map[string][]string)
	for i := range extensions {
		ext := &extensions[i]
		ec.Extensions[i] = ext
		ec.ExtNameMap[ext.Name] = ext
		if ext.Alias != "" && ext.Lead {
			ec.ExtAliasMap[ext.Alias] = ext
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
		if ext.HasSolib && !ext.NeedDDL {
			ctrlLess[ext.Name] = true
		}
	}
	ec.ControlLess = ctrlLess
	return nil
}

// GetDependency returns the dependent extension with the given extensino name
func GetDependency(name string) []string {
	return Catalog.Dependency[name]
}

// LoadAliasMap loads the alias map for the given distribution code
func (ec *ExtensionCatalog) LoadAliasMap(distroCode string) {
	if distroCode == "" {
		distroCode = config.OSType
	}
	ec.AliasMap = map[string]string{}
	switch distroCode {
	case "el", "rpm", "el8", "el9":
		pkgMap := map[string]string{
			"postgresql":    "postgresql$v*",
			"pgsql-common":  "patroni patroni-etcd pgbouncer pgbackrest pg_exporter pgbadger vip-manager",
			"patroni":       "patroni patroni-etcd",
			"pgbouncer":     "pgbouncer",
			"pgbackrest":    "pgbackrest",
			"pg_exporter":   "pg_exporter",
			"vip-manager":   "vip-manager",
			"pgbadger":      "pgbadger",
			"pg_activity":   "pg_activity",
			"pg_filedump":   "pg_filedump",
			"pgxnclient":    "pgxnclient",
			"pgformatter":   "pgformatter",
			"pgcopydb":      "pgcopydb",
			"pgloader":      "pgloader",
			"pg_timetable":  "pg_timetable",
			"wiltondb":      "wiltondb",
			"polardb":       "PolarDB",
			"pig":           "pig",
			"ivorysql":      "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test",
			"ivorysql-all":  "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test ivorysql3-docs ivorysql3-devel ivorysql3-llvmjit",
			"ivorysql3":     "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test",
			"ivorysql3-all": "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test ivorysql3-docs ivorysql3-devel ivorysql3-llvmjit",
			"ivorysql4":     "ivorysql4 ivorysql4-server ivorysql4-contrib ivorysql4-libs ivorysql4-plperl ivorysql4-plpython3 ivorysql4-pltcl ivorysql4-test",
			"ivorysql4-all": "ivorysql4 ivorysql4-server ivorysql4-contrib ivorysql4-libs ivorysql4-plperl ivorysql4-plpython3 ivorysql4-pltcl ivorysql4-test ivorysql4-docs ivorysql4-devel ivorysql4-llvmjit",
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
		for _, ver := range PostgresActiveMajorVersions {
			for k, v := range pkgMapTmpl {
				key := strings.Replace(k, "pgsql", fmt.Sprintf("pg%d", ver), 1)
				value := strings.Replace(v, "$v", fmt.Sprintf("%d", ver), -1)
				pkgMap[key] = value
			}
		}
		ec.AliasMap = pkgMap
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
			"pig":          "pig",
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
		for _, ver := range PostgresActiveMajorVersions {
			for k, v := range pkgMapTmpl {
				key := strings.Replace(k, "pgsql", fmt.Sprintf("pg%d", ver), 1)
				value := strings.Replace(v, "$v", fmt.Sprintf("%d", ver), -1)
				pkgMap[key] = value
			}
		}
		ec.AliasMap = pkgMap
	}

}
