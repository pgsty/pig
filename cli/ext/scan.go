package ext

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// badCaseExtensions is a map of extensions that have bad case in their names
var badCaseExtensions = map[string]bool{
	"address_standardizer-3":         true,
	"address_standardizer_data_us-3": true,
	"postgis-3":                      true,
	"postgis_raster-3":               true,
	"postgis_sfcgal-3":               true,
	"postgis_tiger_geocoder-3":       true,
	"postgis_topology-3":             true,
	"pg_proctab--0.0.10-compat":      true,
}

// lib name to ext name mapping, special case
var matchSpecialCase = map[string]string{
	"pgxml":                         "xml2",
	"_int":                          "intarray",
	"hstore_plpython3":              "hstore_plpython3u",
	"jsonb_plpython3":               "jsonb_plpython3u",
	"libduckdb":                     "pg_duckdb",
	"libhive":                       "hdfs_fdw",
	"llvmjit":                       "llvmjit",
	"pg_partman_bgw":                "pg_partman",
	"pgcryptokey_acpass":            "pgcryptokey",
	"pglogical_output":              "pglogical",
	"pgq_lowlevel":                  "pgq",
	"pgq_triggers":                  "pgq",
	"citus_pgoutput":                "citus",
	"citus_wal2json":                "citus",
	"pgroonga_check":                "pgroonga",
	"pgroonga_crash_safer":          "pgroonga",
	"pgroonga_standby_maintainer":   "pgroonga",
	"pgroonga_wal_applier":          "pgroonga",
	"pgroonga_wal_resource_manager": "pgroonga",
	"plugin_debugger":               "pldbgapi",
	"ddl_deparse":                   "pgl_ddl_deploy",
	"pg_timestamp":                  "pg_bulkload",
	"documentdb_extended_rum":       "documentdb",
	"pg_mooncake_duckdb":            "pg_mooncake",
	"mobilitydb_datagen":            "mobilitydb",
}

var matchGlobCase = map[string]string{
	"libMobilityDB-*": "mobilitydb",
	"libpgrouting-*":  "pgrouting",
	"libpljava-so-*":  "pljava",
	"timescaledb-*":   "timescaledb",
	"pg_documentdb*":  "documentdb",
}

var matchBuiltInLib = map[string]bool{
	"libpqwalreceiver": true,
	"dict_snowball":    true,
	"llvmjit":          true,
	"libecpg":          true,
	"libecpg_compat":   true,
	"libpgtypes":       true,
	"libpq":            true,
}

// ExtensionInstall stores information about an Installed extension
type ExtensionInstall struct {
	*Extension
	Postgres       *PostgresInstall  // Belonged PostgreSQL installation
	InstallVersion string            // Extension name
	ControlName    string            // Control file name
	ControlDesc    string            // Extension description
	ControlMeta    map[string]string // Metadata
	Libraries      map[string]bool   // Associated shared library
}

// ExtName returns the name of the extension
func (e *ExtensionInstall) ExtName() string {
	if e.Extension != nil {
		return e.Extension.Name
	}
	return e.ControlName
}

// Description returns the description of the extension
func (e *ExtensionInstall) Description() string {
	if e.ControlDesc != "" {
		return e.ControlDesc
	} else if e.Extension != nil {
		return e.Extension.EnDesc
	}
	return ""
}

// VersionString returns the version string of the extension
func (e *ExtensionInstall) VersionString() string {
	if e.InstallVersion != "" {
		return e.InstallVersion
	} else if e.Extension != nil {
		return e.Extension.Version
	}
	return ""
}

// ControlPath returns the path to the control file of the extension
func (e *ExtensionInstall) ControlPath() string {
	if e.Postgres == nil || e.Postgres.ExtPath == "" || e.ControlName == "" {
		return ""
	}
	return filepath.Join(e.Postgres.ExtPath, e.ControlName+".control")
}

// ParseControlFile parses the control file of an extension
func (ei *ExtensionInstall) ParseControlFile() error {
	controlPath := ei.ControlPath()
	file, err := os.Open(controlPath)
	if err != nil {
		return err
	}
	defer file.Close()
	ei.ControlMeta = make(map[string]string)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "'")
		switch key {
		case "default_version":
			ei.InstallVersion = value
		case "comment":
			ei.ControlDesc = value
		default:
			ei.ControlMeta[key] = value
		}
	}
	return scanner.Err()
}

// ScanExtensions scans PostgreSQL extensions
func (p *PostgresInstall) ScanExtensions() error {
	// scan shared libraries
	entries, err := os.ReadDir(p.LibPath)
	if err != nil {
		return fmt.Errorf("failed to read shared libraries dir: %v", err)
	}
	shareLibs := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".so") || strings.HasSuffix(entry.Name(), ".dylib")) {
			libName := strings.TrimSuffix(entry.Name(), ".so")
			libName = strings.TrimSuffix(libName, ".dylib")
			shareLibs[libName] = false
		}
	}

	// scan control files
	var extensions []*ExtensionInstall
	extMap := make(map[string]*ExtensionInstall)
	extensionsPath := filepath.Join(p.ExtPath)
	entries, err = os.ReadDir(extensionsPath)
	if err != nil {
		return fmt.Errorf("failed to read extensions dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".control") {
			extName := strings.TrimSuffix(entry.Name(), ".control")
			if badCaseExtensions[extName] {
				continue
			}

			// Normalize extension name for omni extensions (e.g., omni_sqlite--0.2.2 -> omni_sqlite)
			normExtName := extName
			if strings.HasPrefix(extName, "omni") {
				normExtName = strings.SplitN(extName, "--", 2)[0]
			}

			// Skip if normalized name already exists (avoid duplicates for versioned control files)
			if _, alreadyExists := extMap[normExtName]; alreadyExists {
				continue
			}

			extInstall := &ExtensionInstall{Postgres: p, ControlName: extName}
			extMap[extName] = extInstall
			// Also add normalized name to extMap for control-less lookup
			if normExtName != extName {
				extMap[normExtName] = extInstall
			}
			extensions = append(extensions, extInstall)
			extInstall.Libraries = make(map[string]bool, 0)
			_ = extInstall.ParseControlFile()

			// DEPENDENCY: find the extension object in the global Extensions list
			ext := Catalog.ExtNameMap[normExtName]
			if ext == nil {
				logrus.Debugf("failed to find extension %s in catalog", normExtName)
				continue
			} else {
				extInstall.Extension = ext
			}
		}
	}

	// add control less extensions if found
	for name := range Catalog.ControlLess {
		if _, exists := shareLibs[name]; exists {
			// skip if already added from control file
			if _, alreadyExists := extMap[name]; alreadyExists {
				continue
			}
			extInstall := &ExtensionInstall{Postgres: p}
			// DEPENDENCY: find the control less extension in catalog
			extInstall.Extension = Catalog.ExtNameMap[name]
			extInstall.Libraries = map[string]bool{name: true}
			extensions = append(extensions, extInstall)
		}
	}

	// match existing extensions with shared libraries
	for lib := range shareLibs {
		if ext, exists := extMap[lib]; exists {
			ext.Libraries[lib] = true
			shareLibs[lib] = true
		} else {
			for _, ext := range extensions {
				if MatchExtensionWithLibs(ext.ExtName(), lib) {
					ext.Libraries[lib] = true
					shareLibs[lib] = true
				}
			}
		}
	}

	// update extension map
	p.Extensions = extensions
	p.ExtensionMap = extMap
	p.SharedLibs = shareLibs
	return nil
}

func isEncodingLib(libname string) bool {
	if strings.HasPrefix(libname, "euc") || strings.HasPrefix(libname, "utf8") || strings.HasPrefix(libname, "latin") || libname == "cyrillic_and_mic" {
		return true
	}
	return false
}

func isBuiltInLib(libname string) bool {
	if _, exists := matchBuiltInLib[libname]; exists {
		return true
	}
	return false
}

func MatchExtensionWithLibs(extname, libname string) bool {
	// Normalize names for comparison
	normExtname := strings.ToLower(strings.TrimSpace(extname))
	normLibname := strings.ToLower(strings.TrimSpace(libname))

	if normExtname == normLibname {
		return true
	}
	if ename, exists := matchSpecialCase[libname]; exists {
		if extname == ename {
			return true
		}
	}
	for pattern, ename := range matchGlobCase {
		if match, _ := filepath.Match(pattern, libname); match {
			if extname == ename {
				return true
			}
		}
	}
	if libname+"u" == extname || extname+"u" == libname {
		return true
	}
	// remove lib-version... then match
	// Check if libname has a version suffix and remove it
	if idx := strings.LastIndex(libname, "-"); idx != -1 {
		libnameWithoutVersion := libname[:idx]
		if extname == libnameWithoutVersion {
			return true
		}
	}

	return false
}
