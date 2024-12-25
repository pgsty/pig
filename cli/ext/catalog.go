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
}

// DefaultExtensionCatalog creates a new ExtensionCatalog with embedded data which (may) never fails
func DefaultExtensionCatalog() *ExtensionCatalog {
	ec, _ := NewExtensionCatalog()
	_ = ec.Load(embedExtensionData)
	return ec
}

// NewExtensionCatalog creates a new ExtensionCatalog, using embedded data if any error occurs
func NewExtensionCatalog(paths ...string) (*ExtensionCatalog, error) {
	ec := &ExtensionCatalog{}
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

// GetDependency returns the dependant extension with the given extensino name
func GetDependency(name string) []string {
	return Catalog.Dependency[name]
}
