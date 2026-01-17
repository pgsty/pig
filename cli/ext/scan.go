package ext

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"sort"
	"strings"
	"text/tabwriter"

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

func (e *ExtensionInstall) Found() bool {
	return e.Extension != nil
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

// SharedLibraries returns the shared libraries list of the extension
func (e *ExtensionInstall) SharedLibraries() []string {
	var libs []string
	switch config.OSType {
	case config.DistroEL, config.DistroDEB:
		for lib := range e.Libraries {
			libs = append(libs, lib+".so")
		}
	case config.DistroMAC:
		for lib := range e.Libraries {
			libs = append(libs, lib+".dylib")
		}
	}
	return libs
}

// ActiveVersion returns the active version of the extension (fallback to the catalog version)
func (e *ExtensionInstall) ActiveVersion() string {
	if e.InstallVersion != "" {
		return e.InstallVersion
	}
	return e.Version
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

// ExtensionInstallSummary prints a summary of the PostgreSQL installation and its extensions & shared libraries
func (pg *PostgresInstall) ExtensionInstallSummary() {
	// Sort extensions by ID for consistent output
	extensions := pg.Extensions
	sort.Slice(extensions, func(i, j int) bool {
		// Sort by extension ID if available, otherwise by name
		if extensions[i].Extension != nil && extensions[j].Extension != nil {
			return extensions[i].Extension.ID < extensions[j].Extension.ID
		}
		return extensions[i].ExtName() < extensions[j].ExtName()
	})

	// Print extension details including shared libraries in a tabulated format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tVersion\tSharedLibs\tDescription\tMeta")
	fmt.Fprintln(w, "----\t-------\t----------\t---------------------\t------")

	for _, ext := range extensions {
		if !ext.Found() {
			logrus.Warnf("extension %s not found in catalog", ext.ExtName())
		}
		extDescHead := ext.Description()
		if len(extDescHead) > 64 {
			extDescHead = extDescHead[:64] + "..."
		}
		meta := ""
		for k, v := range ext.ControlMeta {
			meta += fmt.Sprintf("%s=%s ", k, v)
		}
		if len(ext.SharedLibraries()) > 0 {
			meta += fmt.Sprintf("lib=%s", strings.Join(ext.SharedLibraries(), ", "))
		}
		meta = strings.TrimSpace(meta)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			ext.ExtName(),
			ext.VersionString(),
			extDescHead,
			meta,
		)

	}
	w.Flush()

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	var unmatchedLibs []string
	var encodingLibs []string
	var builtInLibs []string
	for libName, matched := range pg.SharedLibs {
		if isEncodingLib(libName) {
			encodingLibs = append(encodingLibs, libName)
			continue
		}
		if isBuiltInLib(libName) {
			builtInLibs = append(builtInLibs, libName)
			continue
		}
		if !matched {
			unmatchedLibs = append(unmatchedLibs, libName)
		}
	}
	sort.Strings(encodingLibs)
	sort.Strings(builtInLibs)
	sort.Strings(unmatchedLibs)
	fmt.Printf("\nEncoding Libs: %s\n", strings.Join(encodingLibs, ", "))
	fmt.Printf("\nBuilt-in Libs: %s\n", strings.Join(builtInLibs, ", "))
	if len(unmatchedLibs) > 0 {
		fmt.Printf("\nUnmatched Shared Libraries: %s\n", strings.Join(unmatchedLibs, ", "))
		for _, libName := range unmatchedLibs {
			fmt.Fprintf(w, "%s\n", libName)
		}
	}
	w.Flush()
}

func PrintInstalledPostgres() string {
	if Installs == nil {
		return ""
	}
	var pgVerList []int
	for v := range Installs {
		pgVerList = append(pgVerList, v)
	}

	// sort in reverse
	sort.Sort(sort.Reverse(sort.IntSlice(pgVerList)))
	if len(pgVerList) == 0 {
		return "no installation found"
	}
	if len(pgVerList) == 1 {
		return fmt.Sprintf("%d (active)", pgVerList[0])
	}
	var pgVerStrList []string
	for _, v := range pgVerList {
		if Active != nil && v == Active.MajorVersion {
			pgVerStrList = append(pgVerStrList, fmt.Sprintf("%d (active)", v))
		} else {
			pgVerStrList = append(pgVerStrList, fmt.Sprintf("%d", v))
		}
	}

	return strings.Join(pgVerStrList, ", ")
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
