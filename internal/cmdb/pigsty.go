package cmdb

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pig/internal/utils"

	"github.com/jackc/pgx/v5"
)

const metadbEnvironment = "METADB_URL"

var simpleDatabaseName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// PigstyConnectionOptions follows the existing script convention: an explicit
// libpq target, METADB_URL, then service=meta. LookupEnv is a test seam.
type PigstyConnectionOptions struct {
	Database  string
	LookupEnv func(string) (string, bool)
}

// ResolvePigstyConnection resolves only the existing Pigsty CMDB convention.
// The actual connection value remains private to this package.
func ResolvePigstyConnection(options PigstyConnectionOptions) (ResolvedConnection, error) {
	if value := strings.TrimSpace(options.Database); value != "" {
		if simpleDatabaseName.MatchString(value) {
			value = "dbname=" + value
		}
		return ResolvedConnection{source: ConnectionArgSource, value: value}, nil
	}
	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if value, ok := lookup(metadbEnvironment); ok {
		if strings.TrimSpace(value) == "" {
			return ResolvedConnection{}, failure("CMDB_CONNECTION_INVALID", "METADB_URL is empty")
		}
		return ResolvedConnection{source: ConnectionURLSource, value: value}, nil
	}
	return ResolvedConnection{source: ConnectionMetaSource, value: "service=meta"}, nil
}

// BaselineInspection reports whether init would replace either schema.
type BaselineInspection struct {
	ExistingSchemas int
}

// BaselineResult is value-free verification metadata for exact cmdb.sql use.
type BaselineResult struct {
	StatementResults int
	VerifiedObjects  int
}

var requiredPigstyObjects = []string{
	"pigsty.global_var", "pigsty.group", "pigsty.group_var", "pigsty.host",
	"pigsty.host_var", "pigsty.raw_config", "pigsty.inventory", "pglog.sample",
}

func (s *Store) InspectPigstyBaseline(ctx context.Context) (BaselineInspection, error) {
	if s == nil || s.pool == nil {
		return BaselineInspection{}, failure("CMDB_CONNECTION_INVALID", "database is not open")
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname = ANY($1::text[])`, []string{"pigsty", "pglog"}).Scan(&count); err != nil {
		return BaselineInspection{}, sanitizeDatabaseFailure("CMDB_BASELINE_INSPECTION_FAILED", "baseline inspection", err)
	}
	return BaselineInspection{ExistingSchemas: count}, nil
}

// ApplyPigstyBaseline executes the selected Pigsty files/cmdb.sql as one exact
// simple-query stream. It does not translate the baseline into Go migrations.
func (s *Store) ApplyPigstyBaseline(ctx context.Context, source []byte) (BaselineResult, error) {
	if s == nil || s.pool == nil || len(bytes.TrimSpace(source)) == 0 {
		return BaselineResult{}, failure("CMDB_BASELINE_INVALID", "cmdb.sql is empty or database is not open")
	}
	applyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	connection, err := s.pool.Acquire(applyCtx)
	if err != nil {
		return BaselineResult{}, failure("CMDB_CONNECTION_FAILED", "database connection failed")
	}
	defer connection.Release()
	results, err := connection.Conn().PgConn().Exec(applyCtx, string(source)).ReadAll()
	if err != nil {
		return BaselineResult{}, failure("CMDB_BASELINE_APPLY_FAILED", "cmdb.sql execution failed")
	}
	var verified int
	if err := s.pool.QueryRow(applyCtx, `
SELECT count(*)
  FROM unnest($1::text[]) AS object_name
 WHERE to_regclass(object_name) IS NOT NULL`, requiredPigstyObjects).Scan(&verified); err != nil {
		return BaselineResult{}, failure("CMDB_BASELINE_VERIFY_FAILED", "cmdb.sql object verification failed")
	}
	if verified != len(requiredPigstyObjects) {
		return BaselineResult{}, failure("CMDB_BASELINE_VERIFY_FAILED", "cmdb.sql required objects are incomplete")
	}
	return BaselineResult{StatementResults: len(results), VerifiedObjects: verified}, nil
}

// JSONField is one declaration value already encoded as JSON. Value bytes are
// private input and must never be copied into a Result or error.
type JSONField struct {
	Key   string
	Value json.RawMessage
}

type HostDeclaration struct {
	IP   string
	Vars []JSONField
}

type GroupDeclaration struct {
	Name  string
	Vars  []JSONField
	Hosts []HostDeclaration
}

type Declarations struct {
	Globals []JSONField
	Groups  []GroupDeclaration
}

type LoadResult struct {
	Globals  int
	Groups   int
	Hosts    int
	GroupVar int
	HostVar  int
}

func validateDeclarations(input Declarations) error {
	seenGroups := make(map[string]struct{}, len(input.Groups))
	for _, field := range input.Globals {
		if field.Key == "" || !json.Valid(field.Value) {
			return failure("CMDB_DECLARATION_INVALID", "global declaration is not representable")
		}
	}
	for _, group := range input.Groups {
		if group.Name == "" {
			return failure("CMDB_DECLARATION_INVALID", "group name is empty")
		}
		if _, exists := seenGroups[group.Name]; exists {
			return failure("CMDB_DECLARATION_INVALID", "group is declared more than once")
		}
		seenGroups[group.Name] = struct{}{}
		for _, field := range group.Vars {
			if field.Key == "" || !json.Valid(field.Value) {
				return failure("CMDB_DECLARATION_INVALID", "group declaration is not representable")
			}
		}
		seenHosts := make(map[string]struct{}, len(group.Hosts))
		for _, host := range group.Hosts {
			if !validInet(host.IP) {
				return failure("CMDB_DECLARATION_INVALID", "host key is not a PostgreSQL INET value")
			}
			if _, exists := seenHosts[host.IP]; exists {
				return failure("CMDB_DECLARATION_INVALID", "host is duplicated within a group")
			}
			seenHosts[host.IP] = struct{}{}
			for _, field := range host.Vars {
				if field.Key == "" || !json.Valid(field.Value) {
					return failure("CMDB_DECLARATION_INVALID", "host declaration is not representable")
				}
			}
		}
	}
	return nil
}

func validInet(value string) bool {
	if _, err := netip.ParseAddr(value); err == nil {
		return true
	}
	_, err := netip.ParsePrefix(value)
	return err == nil
}

// ReplaceDeclarations mirrors bin/inventory_load's five-table replacement in
// one transaction. Any failure rolls the truncate and all inserts back.
func (s *Store) ReplaceDeclarations(ctx context.Context, input Declarations) (LoadResult, error) {
	if s == nil || s.pool == nil {
		return LoadResult{}, failure("CMDB_CONNECTION_INVALID", "database is not open")
	}
	if err := validateDeclarations(input); err != nil {
		return LoadResult{}, err
	}
	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{})
	if err != nil {
		return LoadResult{}, failure("CMDB_LOAD_FAILED", "Inventory transaction could not start")
	}
	defer tx.Rollback(txCtx) //nolint:errcheck
	if _, err := tx.Exec(txCtx, `TRUNCATE pigsty.group, pigsty.host, pigsty.group_var, pigsty.host_var, pigsty.global_var`); err != nil {
		return LoadResult{}, failure("CMDB_LOAD_FAILED", "Inventory tables could not be replaced")
	}

	result := LoadResult{}
	for _, field := range input.Globals {
		if _, err := tx.Exec(txCtx, `INSERT INTO pigsty.global_var (key, value) VALUES ($1, $2::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, mtime = now()`, field.Key, string(field.Value)); err != nil {
			return LoadResult{}, failure("CMDB_LOAD_FAILED", "global declarations could not be loaded")
		}
		result.Globals++
	}
	for _, group := range input.Groups {
		if _, err := tx.Exec(txCtx, `INSERT INTO pigsty.group (cls) VALUES ($1) ON CONFLICT (cls) DO UPDATE SET mtime = now()`, group.Name); err != nil {
			return LoadResult{}, failure("CMDB_LOAD_FAILED", "groups could not be loaded")
		}
		result.Groups++
		for _, field := range group.Vars {
			if _, err := tx.Exec(txCtx, `INSERT INTO pigsty.group_var (cls, key, value) VALUES ($1, $2, $3::jsonb) ON CONFLICT (cls, key) DO UPDATE SET value = EXCLUDED.value, mtime = now()`, group.Name, field.Key, string(field.Value)); err != nil {
				return LoadResult{}, failure("CMDB_LOAD_FAILED", "group variables could not be loaded")
			}
			result.GroupVar++
		}
		for _, host := range group.Hosts {
			if _, err := tx.Exec(txCtx, `INSERT INTO pigsty.host (cls, ip) VALUES ($1, $2::inet) ON CONFLICT (cls, ip) DO UPDATE SET mtime = now()`, group.Name, host.IP); err != nil {
				return LoadResult{}, failure("CMDB_LOAD_FAILED", "hosts could not be loaded")
			}
			result.Hosts++
			for _, field := range host.Vars {
				if _, err := tx.Exec(txCtx, `INSERT INTO pigsty.host_var (cls, ip, key, value) VALUES ($1, $2::inet, $3, $4::jsonb) ON CONFLICT (cls, ip, key) DO UPDATE SET value = EXCLUDED.value, mtime = now()`, group.Name, host.IP, field.Key, string(field.Value)); err != nil {
					return LoadResult{}, failure("CMDB_LOAD_FAILED", "host variables could not be loaded")
				}
				result.HostVar++
			}
		}
	}
	if err := tx.Commit(txCtx); err != nil {
		return LoadResult{}, failure("CMDB_LOAD_FAILED", "Inventory transaction could not commit")
	}
	return result, nil
}

type TextField struct {
	Key   string
	Value string
}

type OSDefaults struct {
	Values   []JSONField
	Packages []TextField
}

type DefaultsResult struct {
	Values   int
	Packages int
}

// ApplyOSDefaults mirrors the loader's separate best-effort default update.
func (s *Store) ApplyOSDefaults(ctx context.Context, input OSDefaults) (DefaultsResult, error) {
	if s == nil || s.pool == nil {
		return DefaultsResult{}, failure("CMDB_CONNECTION_INVALID", "database is not open")
	}
	if len(input.Values) == 0 && len(input.Packages) == 0 {
		return DefaultsResult{}, nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default transaction could not start")
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	result := DefaultsResult{}
	for _, field := range input.Values {
		if field.Key == "" || !json.Valid(field.Value) {
			return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default value is invalid")
		}
		tag, err := tx.Exec(ctx, `UPDATE pigsty.default_var SET value = $1::jsonb WHERE key = $2`, string(field.Value), field.Key)
		if err != nil {
			return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default values could not be updated")
		}
		result.Values += int(tag.RowsAffected())
	}
	if len(input.Packages) > 0 {
		if _, err := tx.Exec(ctx, `TRUNCATE pigsty.package_map`); err != nil {
			return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "package map could not be replaced")
		}
		for _, field := range input.Packages {
			if field.Key == "" {
				return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "package map key is empty")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO pigsty.package_map (key, value) VALUES ($1, $2)`, field.Key, field.Value); err != nil {
				return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "package map could not be loaded")
			}
			result.Packages++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default transaction could not commit")
	}
	return result, nil
}

func (s *Store) RawConfig(ctx context.Context) ([]byte, error) {
	if s == nil || s.pool == nil {
		return nil, failure("CMDB_CONNECTION_INVALID", "database is not open")
	}
	var body []byte
	if err := s.pool.QueryRow(ctx, `SELECT jsonb_build_object FROM pigsty.raw_config`).Scan(&body); err != nil || !json.Valid(body) {
		return nil, failure("CMDB_DUMP_FAILED", "raw_config could not be read")
	}
	return bytes.Clone(body), nil
}

func (s *Store) DynamicInventory(ctx context.Context) ([]byte, error) {
	if s == nil || s.pool == nil {
		return nil, failure("CMDB_CONNECTION_INVALID", "database is not open")
	}
	var body []byte
	if err := s.pool.QueryRow(ctx, `SELECT text FROM pigsty.inventory`).Scan(&body); err != nil || !json.Valid(body) {
		return nil, failure("CMDB_INVENTORY_FAILED", "dynamic Inventory could not be read")
	}
	return bytes.Clone(body), nil
}

type PigstySourceResult struct {
	ConfigPath    string
	InventoryPath string
	Changed       bool
}

// EnablePigstyInventory writes the same query adapter as inventory_cmdb and
// switches exactly one [defaults] inventory directive to inventory.sh.
func (s *Store) EnablePigstyInventory(ctx context.Context, home string) (PigstySourceResult, error) {
	if _, err := s.DynamicInventory(ctx); err != nil {
		return PigstySourceResult{}, err
	}
	if literalConnectionHasPassword(s.connection) {
		return PigstySourceResult{}, failure("CMDB_CONNECTION_SECRET", "inventory.sh cannot contain a password-bearing connection literal")
	}
	home, err := filepath.Abs(strings.TrimSpace(home))
	if err != nil || home == "" {
		return PigstySourceResult{}, failure("CMDB_SOURCE_INVALID", "Pigsty home is invalid")
	}
	home = filepath.Clean(home)
	inventoryPath := filepath.Join(home, "inventory.sh")
	configPath := filepath.Join(home, "ansible.cfg")
	body := []byte("#!/bin/bash\nexec psql " + shellQuote(s.connection) + " -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if err := utils.AtomicWriteFile(inventoryPath, body, 0o755); err != nil {
		return PigstySourceResult{}, failure("CMDB_SWITCH_FAILED", "inventory.sh could not be written")
	}
	changed, err := switchInventoryLiteral(configPath, "inventory.sh", inventoryPath)
	if err != nil {
		return PigstySourceResult{}, err
	}
	return PigstySourceResult{ConfigPath: configPath, InventoryPath: inventoryPath, Changed: changed}, nil
}

// DisablePigstyInventory mirrors inventory_conf without touching the database
// or deleting inventory.sh.
func DisablePigstyInventory(home string) (PigstySourceResult, error) {
	home, err := filepath.Abs(strings.TrimSpace(home))
	if err != nil || home == "" {
		return PigstySourceResult{}, failure("CMDB_SOURCE_INVALID", "Pigsty home is invalid")
	}
	home = filepath.Clean(home)
	configPath := filepath.Join(home, "ansible.cfg")
	inventoryPath := filepath.Join(home, "pigsty.yml")
	changed, err := switchInventoryLiteral(configPath, "pigsty.yml", inventoryPath)
	if err != nil {
		return PigstySourceResult{}, err
	}
	return PigstySourceResult{ConfigPath: configPath, InventoryPath: inventoryPath, Changed: changed}, nil
}

func switchInventoryLiteral(configPath, literal, target string) (bool, error) {
	document, err := readAnsibleConfig(configPath)
	if err != nil {
		return false, err
	}
	patch, err := planAnsibleConfigLiteralPatch(document, literal, target)
	if err != nil {
		return false, err
	}
	if bytes.Equal(document.body, patch.candidate) {
		return false, nil
	}
	if err := commitAnsibleConfigPatch(patch); err != nil {
		return false, err
	}
	return true, nil
}

func literalConnectionHasPassword(value string) bool {
	trimmed := strings.TrimSpace(value)
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.User != nil {
		if _, exists := parsed.User.Password(); exists {
			return true
		}
	}
	lower := strings.ToLower(trimmed)
	for offset := 0; ; {
		index := strings.Index(lower[offset:], "password")
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || lower[index-1] == ' ' || lower[index-1] == '\t'
		after := strings.TrimLeft(lower[index+len("password"):], " \t")
		if beforeOK && strings.HasPrefix(after, "=") {
			return true
		}
		offset = index + len("password")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
