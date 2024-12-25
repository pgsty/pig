package ext

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
			shareLibs[libName] = true
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
			extInstall := &ExtensionInstall{Postgres: p, ControlName: extName}
			extMap[extName] = extInstall
			extensions = append(extensions, extInstall)

			extInstall.Libraries = make(map[string]bool, 0)
			_ = extInstall.ParseControlFile()
			// DEPENDENCY: find the extension object in the global Extensions list
			ext := Catalog.ExtNameMap[extName]
			if ext == nil {
				logrus.Debugf("failed to find extension %s in catalog", extName)
				continue
			} else {
				extInstall.Extension = ext
			}
		}
	}

	// match existing extensions with shared libraries
	for lib := range shareLibs {
		if ext, exists := extMap[lib]; exists {
			ext.Libraries[lib] = true
		} else {
			for _, ext := range extensions {
				if MatchExtensionWithLibs(ext.ExtName(), lib) {
					ext.Libraries[lib] = true
				}
			}
		}
	}

	// add control less extensions if found
	for name := range Catalog.ControlLess {
		if _, exists := shareLibs[name]; exists {
			extInstall := &ExtensionInstall{Postgres: p}
			// DEPENDENCY: find the control less extension in catalog
			extInstall.Extension = Catalog.ExtNameMap[name]
			extInstall.Libraries = map[string]bool{name: true}
			extensions = append(extensions, extInstall)
		}
	}

	// update extension map
	p.Extensions = extensions
	p.ExtensionMap = extMap
	return nil
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

// // PostgresInstallSummary print the summary of PostgreSQL installation
// func PostgresInstallSummary() {
// 	if !Inited {
// 		fmt.Printf("PostgreSQL Environment not initialized\n")
// 		return
// 	}

// 	// print installed PostgreSQL versions
// 	if len(Installs) > 0 {
// 		fmt.Printf("Installed:\n")
// 		for _, v := range Installs {
// 			if v == Active {
// 				fmt.Printf("* %-17s\t%s\n", fmt.Sprintf("%d.%d", v.MajorVersion, v.MinorVersion), v.PgConfig)
// 			}
// 		}
// 		for _, v := range Installs {
// 			if v != Active {
// 				fmt.Printf("- %-15s\t%s\n", fmt.Sprintf("%d.%d", v.MajorVersion, v.MinorVersion), v.PgConfig)
// 			}
// 		}
// 	} else {
// 		fmt.Println("No PostgreSQL installation found")
// 	}

// 	// print active PostgreSQL detail
// 	if Active != nil {
// 		fmt.Printf("\nActive:\n")
// 		fmt.Printf("PG Version        :  %s\n", Active.Version)
// 		fmt.Printf("Config Path       :  %s\n", Active.PgConfig)
// 		fmt.Printf("Binary Path       :  %s\n", Active.BinPath)
// 		fmt.Printf("Library Path      :  %s\n", Active.LibPath)
// 		fmt.Printf("Extension Path    :  %s\n", Active.ExtPath)
// 		if len(Active.Extensions) > 0 {
// 			fmt.Printf("Extension Stat    :  Installed %d\n", len(Active.Extensions))
// 		}
// 	} else {
// 		fmt.Println("No PostgreSQL installation activated")
// 		fmt.Printf("PATH: %s\n", os.Getenv("PATH"))
// 	}
// }

// // GetPostgres returns the active PostgreSQL installation (via pg_config path or major version)
// func GetPostgres(path string, ver int) (pg *PostgresInstall, err error) {
// 	if path != "" {
// 		return DetectPostgresFromConfig(path)
// 	}
// 	if !Inited {
// 		err = DetectPostgresEnv()
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
// 	if ver != 0 {
// 		if pg, exists := Installs[ver]; exists {
// 			return pg, nil
// 		} else {
// 			return nil, fmt.Errorf("PostgreSQL version %d is not installed", ver)
// 		}
// 	}
// 	if Active == nil {
// 		return nil, fmt.Errorf("no active PostgreSQL installation detected")
// 	} else {
// 		return Active, nil
// 	}
// }

func MatchExtensionWithLibs(extname, libname string) bool {
	if extname == libname {
		return true
	}
	return false
}
