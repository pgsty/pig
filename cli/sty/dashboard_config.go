package sty

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"pig/internal/grafana"
	"pig/inventory"
)

const maxDashboardPasswordBytes = 64 << 10

// DashboardOptions contains connection, repository, and action scope. Secrets
// are consumed during preparation and are never copied into command DTOs.
type DashboardOptions struct {
	Home         string
	Inventory    string
	Manifest     string
	Endpoint     string
	Username     string
	PasswordFile string
	DumpDir      string
	Prune        bool
	Yes          bool
	HTTPClient   *http.Client
}

type dashboardPrepared struct {
	manifest  *dashboardManifestSource
	client    *grafana.Client
	inventory *inventory.Document
}

func prepareDashboard(options DashboardOptions, requireAssets bool) (*dashboardPrepared, error) {
	manifest, err := loadDashboardManifest(options.Home, options.Manifest)
	if err != nil {
		return nil, err
	}
	if requireAssets {
		if _, err := loadDashboardRepositoryAssets(manifest); err != nil {
			return nil, err
		}
	}
	doc, err := loadDashboardCredentialInventory(options.Inventory)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("GRAFANA_ENDPOINT"))
	}
	if endpoint == "" {
		port := 3000
		if declared, ok := dashboardInventoryInteger(doc, "grafana_port"); ok && declared > 0 && declared <= 65535 {
			port = declared
		}
		endpoint = "http://127.0.0.1:" + strconv.Itoa(port) + "/ui"
	}
	username := strings.TrimSpace(options.Username)
	if username == "" {
		username = strings.TrimSpace(os.Getenv("GRAFANA_USERNAME"))
	}
	if username == "" {
		username, _ = dashboardInventoryString(doc, "grafana_admin_username")
	}
	if username == "" {
		username = "admin"
	}
	password, err := dashboardAPIPassword(options.PasswordFile, doc)
	if err != nil {
		return nil, err
	}
	if password == "" {
		return nil, errors.New("Grafana password is not available from --password-file, GRAFANA_PASSWORD, or all.vars.grafana_admin_password")
	}
	client, err := grafana.NewClient(grafana.Config{
		Endpoint: endpoint, Username: username, Password: password, HTTPClient: options.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &dashboardPrepared{manifest: manifest, client: client, inventory: doc}, nil
}

func loadDashboardCredentialInventory(filename string) (*inventory.Document, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, nil
	}
	resolved, source, failure := readStaticInventorySource(filename, 0)
	if failure != nil {
		return nil, errors.New("active Inventory could not be read for Grafana credential resolution")
	}
	doc, err := inventory.Parse(resolved, source)
	if err != nil {
		return nil, errors.New("active Inventory could not be parsed for Grafana credential resolution")
	}
	return doc, nil
}

func dashboardAPIPassword(passwordFile string, doc *inventory.Document) (string, error) {
	passwordFile = strings.TrimSpace(passwordFile)
	if passwordFile != "" {
		absolute, err := filepath.Abs(passwordFile)
		if err != nil {
			return "", fmt.Errorf("resolve Grafana password file: %w", err)
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return "", errors.New("Grafana password file is unavailable")
		}
		if !info.Mode().IsRegular() || info.Size() > maxDashboardPasswordBytes {
			return "", errors.New("Grafana password file must be one bounded regular file")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return "", errors.New("Grafana password file must be owner-only")
		}
		raw, err := os.ReadFile(absolute)
		if err != nil {
			return "", errors.New("Grafana password file could not be read")
		}
		password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		if strings.ContainsAny(password, "\x00\r\n") {
			return "", errors.New("Grafana password file must contain one credential line")
		}
		return password, nil
	}
	if password := os.Getenv("GRAFANA_PASSWORD"); password != "" {
		return password, nil
	}
	password, _ := dashboardInventoryString(doc, "grafana_admin_password")
	return password, nil
}

func dashboardDatasourcePayload(item DashboardManifestDatasource, doc *inventory.Document, creating bool) (map[string]interface{}, bool, error) {
	payload, err := desiredDatasourceValue(item)
	if err != nil {
		return nil, false, err
	}
	secure := make(map[string]interface{})
	for _, field := range item.SecureFields {
		value := ""
		if field.Env != "" {
			value = os.Getenv(field.Env)
		}
		if value == "" && field.Inventory != "" {
			value, _ = dashboardInventoryString(doc, field.Inventory)
		}
		if value == "" {
			if creating && field.RequiredOnCreate {
				return nil, false, fmt.Errorf("datasource %s requires protected field %s when created", item.UID, field.Name)
			}
			continue
		}
		secure[field.Name] = value
	}
	if len(secure) > 0 {
		payload["secureJsonData"] = secure
	}
	return payload, len(secure) > 0, nil
}

func dashboardInventoryString(doc *inventory.Document, key string) (string, bool) {
	if doc == nil {
		return "", false
	}
	value, ok := doc.Lookup(inventory.AllVars(key))
	if !ok || value == nil || value.Kind != inventory.ScalarValue || value.Scalar == nil || value.Scalar.Kind != inventory.StringScalar {
		return "", false
	}
	if value.Alias != "" || value.Tag != "" && !strings.HasPrefix(value.Tag, "!!") && !strings.HasPrefix(value.Tag, "tag:yaml.org,2002:") {
		return "", false
	}
	text, ok := value.Scalar.Value.(string)
	return text, ok && text != ""
}

func dashboardInventoryInteger(doc *inventory.Document, key string) (int, bool) {
	if doc == nil {
		return 0, false
	}
	value, ok := doc.Lookup(inventory.AllVars(key))
	if !ok || value == nil || value.Kind != inventory.ScalarValue || value.Scalar == nil || value.Scalar.Kind != inventory.IntegerScalar {
		return 0, false
	}
	switch number := value.Scalar.Value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case uint64:
		return int(number), uint64(int(number)) == number
	default:
		parsed, err := strconv.Atoi(fmt.Sprint(number))
		return parsed, err == nil
	}
}
