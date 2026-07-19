package cmdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pig/internal/config"
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

// CanonicalDeclarationsJSON renders the declaration subset represented by the
// existing five Pigsty CMDB tables. JSON object ordering and YAML presentation
// are intentionally discarded; declaration values remain private bytes.
func CanonicalDeclarationsJSON(input Declarations) ([]byte, error) {
	if err := validateDeclarations(input); err != nil {
		return nil, err
	}
	vars := make(map[string]json.RawMessage, len(input.Globals))
	for _, field := range input.Globals {
		value, err := canonicalJSONValue(field.Value)
		if err != nil {
			return nil, failure("CMDB_DECLARATION_INVALID", "global declaration value could not be canonicalized")
		}
		vars[field.Key] = value
	}
	children := make(map[string]any, len(input.Groups))
	for _, group := range input.Groups {
		groupVars := make(map[string]json.RawMessage, len(group.Vars))
		for _, field := range group.Vars {
			value, err := canonicalJSONValue(field.Value)
			if err != nil {
				return nil, failure("CMDB_DECLARATION_INVALID", "group declaration value could not be canonicalized")
			}
			groupVars[field.Key] = value
		}
		hosts := make(map[string]any, len(group.Hosts))
		for _, host := range group.Hosts {
			hostVars := make(map[string]json.RawMessage, len(host.Vars))
			for _, field := range host.Vars {
				value, err := canonicalJSONValue(field.Value)
				if err != nil {
					return nil, failure("CMDB_DECLARATION_INVALID", "host declaration value could not be canonicalized")
				}
				hostVars[field.Key] = value
			}
			hosts[host.IP] = hostVars
		}
		children[group.Name] = map[string]any{"hosts": hosts, "vars": groupVars}
	}
	body, err := json.Marshal(map[string]any{"all": map[string]any{"children": children, "vars": vars}})
	if err != nil {
		return nil, failure("CMDB_DECLARATION_INVALID", "declarations could not be canonicalized")
	}
	return body, nil
}

// CanonicalRawConfigJSON validates and normalizes the exact declaration shape
// emitted by pigsty.raw_config. Null declaration containers become empty JSON
// objects so an empty table and an explicit empty mapping compare equally.
func CanonicalRawConfigJSON(source []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil || !onlyObjectKeys(root, "all") {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config is not one declaration object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config contains trailing data")
	}
	all, ok := root["all"].(map[string]any)
	if !ok || !onlyObjectKeys(all, "children", "vars") {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config all declaration is invalid")
	}
	vars, ok := nullableObject(all["vars"])
	if !ok {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config global declarations are invalid")
	}
	children, ok := nullableObject(all["children"])
	if !ok {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config group declarations are invalid")
	}
	all["vars"], all["children"] = vars, children
	for name, raw := range children {
		group, ok := raw.(map[string]any)
		if name == "" || !ok || !onlyObjectKeys(group, "hosts", "vars") {
			return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config group declaration is invalid")
		}
		groupVars, ok := nullableObject(group["vars"])
		if !ok {
			return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config group variables are invalid")
		}
		hosts, ok := nullableObject(group["hosts"])
		if !ok {
			return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config hosts are invalid")
		}
		for address, rawVars := range hosts {
			if !validInet(address) {
				return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config host key is invalid")
			}
			if _, ok := rawVars.(map[string]any); !ok {
				return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config host variables are invalid")
			}
		}
		group["vars"], group["hosts"] = groupVars, hosts
	}
	if err := normalizeJSONNumbers(root); err != nil {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config contains an invalid number")
	}
	body, err := json.Marshal(root)
	if err != nil {
		return nil, failure("CMDB_RAW_CONFIG_INVALID", "raw_config could not be canonicalized")
	}
	return body, nil
}

func canonicalJSONValue(source json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON value contains trailing data")
	}
	if number, ok := value.(json.Number); ok {
		canonical, err := canonicalJSONNumber(number)
		if err != nil {
			return nil, err
		}
		value = canonical
	} else if err := normalizeJSONNumbers(value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func normalizeJSONNumbers(value any) error {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if number, ok := child.(json.Number); ok {
				canonical, err := canonicalJSONNumber(number)
				if err != nil {
					return err
				}
				current[key] = canonical
				continue
			}
			if err := normalizeJSONNumbers(child); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range current {
			if number, ok := child.(json.Number); ok {
				canonical, err := canonicalJSONNumber(number)
				if err != nil {
					return err
				}
				current[index] = canonical
				continue
			}
			if err := normalizeJSONNumbers(child); err != nil {
				return err
			}
		}
	case json.Number:
		// Root declaration values are stored beneath a key before this walker is
		// called, so there is no parent slot to replace here.
		_, err := canonicalJSONNumber(current)
		return err
	}
	return nil
}

func canonicalJSONNumber(number json.Number) (json.Number, error) {
	text := number.String()
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, text = "-", text[1:]
	}
	mantissa, exponentText, hasExponent := text, "", false
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		mantissa, exponentText, hasExponent = text[:index], text[index+1:], true
	}
	exponent := int64(0)
	if hasExponent {
		parsed, err := strconv.ParseInt(exponentText, 10, 32)
		if err != nil || parsed < -1_000_000 || parsed > 1_000_000 {
			return "", errors.New("JSON number exponent is out of bounds")
		}
		exponent = parsed
	}
	integer, fraction := mantissa, ""
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		integer, fraction = mantissa[:index], mantissa[index+1:]
	}
	digits := integer + fraction
	leading := len(digits) - len(strings.TrimLeft(digits, "0"))
	if leading == len(digits) {
		return json.Number("0"), nil
	}
	digits = digits[leading:]
	digits = strings.TrimRight(digits, "0")
	decimalPosition := int64(len(integer)-leading) + exponent
	scientificExponent := decimalPosition - 1
	mantissa = digits[:1]
	if len(digits) > 1 {
		mantissa += "." + digits[1:]
	}
	if scientificExponent != 0 {
		mantissa += "e" + strconv.FormatInt(scientificExponent, 10)
	}
	return json.Number(sign + mantissa), nil
}

func nullableObject(value any) (map[string]any, bool) {
	if value == nil {
		return map[string]any{}, true
	}
	object, ok := value.(map[string]any)
	return object, ok
}

func onlyObjectKeys(value map[string]any, keys ...string) bool {
	if len(value) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, exists := value[key]; !exists {
			return false
		}
	}
	return true
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
	if !validJSONFields(input.Globals) {
		return failure("CMDB_DECLARATION_INVALID", "global declaration is not representable")
	}
	for _, group := range input.Groups {
		if group.Name == "" {
			return failure("CMDB_DECLARATION_INVALID", "group name is empty")
		}
		if _, exists := seenGroups[group.Name]; exists {
			return failure("CMDB_DECLARATION_INVALID", "group is declared more than once")
		}
		seenGroups[group.Name] = struct{}{}
		if !validJSONFields(group.Vars) {
			return failure("CMDB_DECLARATION_INVALID", "group declaration is not representable")
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
			if !validJSONFields(host.Vars) {
				return failure("CMDB_DECLARATION_INVALID", "host declaration is not representable")
			}
		}
	}
	return nil
}

func validJSONFields(fields []JSONField) bool {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.Key == "" || !json.Valid(field.Value) {
			return false
		}
		if _, exists := seen[field.Key]; exists {
			return false
		}
		seen[field.Key] = struct{}{}
	}
	return true
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
	return s.replaceDeclarations(ctx, input, nil)
}

// ReplaceDeclarationsIfCurrent serializes declaration replacement against the
// canonical raw_config observed by the caller. The five declaration tables are
// locked before the comparison and stay locked through commit.
func (s *Store) ReplaceDeclarationsIfCurrent(ctx context.Context, input Declarations, expectedCanonical []byte) (LoadResult, error) {
	if len(expectedCanonical) == 0 {
		return LoadResult{}, failure("CMDB_SOURCE_CHANGED", "expected CMDB declaration revision is empty")
	}
	return s.replaceDeclarations(ctx, input, bytes.Clone(expectedCanonical))
}

func (s *Store) replaceDeclarations(ctx context.Context, input Declarations, expectedCanonical []byte) (LoadResult, error) {
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
	if len(expectedCanonical) > 0 {
		if _, err := tx.Exec(txCtx, `LOCK TABLE pigsty.global_var, pigsty.group, pigsty.group_var, pigsty.host, pigsty.host_var IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return LoadResult{}, failure("CMDB_LOAD_FAILED", "Inventory tables could not be locked")
		}
		var raw []byte
		if err := tx.QueryRow(txCtx, `SELECT jsonb_build_object FROM pigsty.raw_config`).Scan(&raw); err != nil {
			return LoadResult{}, failure("CMDB_RAW_CONFIG_INVALID", "raw_config could not be read during replacement")
		}
		current, err := CanonicalRawConfigJSON(raw)
		if err != nil {
			return LoadResult{}, err
		}
		if !bytes.Equal(current, expectedCanonical) {
			return LoadResult{}, failure("CMDB_SOURCE_CHANGED", "CMDB declarations changed after planning")
		}
	}
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
	txCtx, cancel := s.transactionContext(ctx)
	defer cancel()
	tx, err := s.pool.BeginTx(txCtx, pgx.TxOptions{})
	if err != nil {
		return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default transaction could not start")
	}
	defer tx.Rollback(txCtx) //nolint:errcheck
	result := DefaultsResult{}
	for _, field := range input.Values {
		if field.Key == "" || !json.Valid(field.Value) {
			return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default value is invalid")
		}
		tag, err := tx.Exec(txCtx, `UPDATE pigsty.default_var SET value = $1::jsonb WHERE key = $2`, string(field.Value), field.Key)
		if err != nil {
			return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "OS-default values could not be updated")
		}
		result.Values += int(tag.RowsAffected())
	}
	if len(input.Packages) > 0 {
		if _, err := tx.Exec(txCtx, `TRUNCATE pigsty.package_map`); err != nil {
			return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "package map could not be replaced")
		}
		for _, field := range input.Packages {
			if field.Key == "" {
				return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "package map key is empty")
			}
			if _, err := tx.Exec(txCtx, `INSERT INTO pigsty.package_map (key, value) VALUES ($1, $2)`, field.Key, field.Value); err != nil {
				return DefaultsResult{}, failure("CMDB_DEFAULTS_FAILED", "package map could not be loaded")
			}
			result.Packages++
		}
	}
	if err := tx.Commit(txCtx); err != nil {
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

// PigstyInspection is safe structural metadata plus the private canonical
// raw_config revision needed by Check and Load orchestration.
type PigstyInspection struct {
	VerifiedObjects int
	Globals         int
	Groups          int
	Hosts           int
	GroupVars       int
	HostVars        int
	CanonicalRaw    []byte
}

// InspectPigstyCMDB verifies the existing schema, declaration projection, and
// dynamic Inventory projection without mutating the database.
func (s *Store) InspectPigstyCMDB(ctx context.Context) (PigstyInspection, error) {
	if s == nil || s.pool == nil {
		return PigstyInspection{}, failure("CMDB_CONNECTION_INVALID", "database is not open")
	}
	inspection := PigstyInspection{}
	if err := s.pool.QueryRow(ctx, `
SELECT count(*)
  FROM unnest($1::text[]) AS object_name
 WHERE to_regclass(object_name) IS NOT NULL`, requiredPigstyObjects).Scan(&inspection.VerifiedObjects); err != nil {
		return PigstyInspection{}, sanitizeDatabaseFailure("CMDB_BASELINE_INSPECTION_FAILED", "schema inspection", err)
	}
	if inspection.VerifiedObjects != len(requiredPigstyObjects) {
		return PigstyInspection{}, failure("CMDB_BASELINE_VERIFY_FAILED", "required Pigsty CMDB objects are incomplete")
	}
	if err := s.pool.QueryRow(ctx, `
SELECT (SELECT count(*) FROM pigsty.global_var),
       (SELECT count(*) FROM pigsty.group),
       (SELECT count(*) FROM pigsty.host),
       (SELECT count(*) FROM pigsty.group_var),
       (SELECT count(*) FROM pigsty.host_var)`).Scan(
		&inspection.Globals, &inspection.Groups, &inspection.Hosts,
		&inspection.GroupVars, &inspection.HostVars,
	); err != nil {
		return PigstyInspection{}, sanitizeDatabaseFailure("CMDB_BASELINE_INSPECTION_FAILED", "declaration inspection", err)
	}
	raw, err := s.RawConfig(ctx)
	if err != nil {
		return PigstyInspection{}, err
	}
	inspection.CanonicalRaw, err = CanonicalRawConfigJSON(raw)
	if err != nil {
		return PigstyInspection{}, err
	}
	dynamic, err := s.DynamicInventory(ctx)
	if err != nil {
		return PigstyInspection{}, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(dynamic, &root); err != nil || root == nil {
		return PigstyInspection{}, failure("CMDB_INVENTORY_FAILED", "dynamic Inventory is not one JSON object")
	}
	var meta map[string]json.RawMessage
	if rawMeta, exists := root["_meta"]; !exists || json.Unmarshal(rawMeta, &meta) != nil || meta == nil {
		return PigstyInspection{}, failure("CMDB_INVENTORY_FAILED", "dynamic Inventory is missing _meta")
	}
	return inspection, nil
}

type PigstySourceResult struct {
	ConfigPath    string
	InventoryPath string
	Changed       bool
	ScriptChanged bool
	ConfigChanged bool
}

// PigstySourcePlan retains private candidate bytes and source revisions inside
// the adapter while exposing only safe file/change metadata to callers.
type PigstySourcePlan struct {
	ConfigPath    string
	InventoryPath string
	Changed       bool
	ScriptChanged bool
	ConfigChanged bool

	scriptBody        []byte
	inventoryBefore   []byte
	inventoryMode     os.FileMode
	inventoryOwnerUID uint32
	inventoryOwnerGID uint32
	inventoryExists   bool
	configPatch       *ansibleConfigPatch
	commitConfig      func(*ansibleConfigPatch) error
}

// EnablePigstyInventory writes the same query adapter as inventory_cmdb and
// switches exactly one [defaults] inventory directive to inventory.sh.
func (s *Store) EnablePigstyInventory(ctx context.Context, home string) (PigstySourceResult, error) {
	plan, err := s.PlanEnablePigstyInventory(ctx, home)
	if err != nil {
		return PigstySourceResult{}, err
	}
	return plan.Apply()
}

// PlanEnablePigstyInventory performs database and local-file preflights without
// writing either inventory.sh or ansible.cfg.
func (s *Store) PlanEnablePigstyInventory(ctx context.Context, home string) (*PigstySourcePlan, error) {
	if _, err := s.InspectPigstyCMDB(ctx); err != nil {
		return nil, err
	}
	home, err := normalizedPigstyHome(home)
	if err != nil {
		return nil, err
	}
	inventoryPath := filepath.Join(home, "inventory.sh")
	configPath := filepath.Join(home, "ansible.cfg")
	body, err := renderPigstyInventoryScript(s.connection)
	if err != nil {
		return nil, err
	}
	document, err := readAnsibleConfig(configPath)
	if err != nil {
		return nil, err
	}
	patch, configChanged, err := planGuardedInventoryLiteral(document, home, "inventory.sh")
	if err != nil {
		return nil, err
	}
	plan := &PigstySourcePlan{
		ConfigPath: configPath, InventoryPath: inventoryPath, scriptBody: bytes.Clone(body),
		ConfigChanged: configChanged, configPatch: patch,
	}
	if err := inspectManagedInventoryScript(plan); err != nil {
		return nil, err
	}
	plan.Changed = plan.ScriptChanged || plan.ConfigChanged
	return plan, nil
}

func renderPigstyInventoryScript(connection string) ([]byte, error) {
	if literalConnectionHasPassword(connection) {
		return nil, failure("CMDB_CONNECTION_SECRET", "inventory.sh cannot contain a password-bearing connection literal")
	}
	body := []byte("#!/bin/bash\nexec psql " + shellQuote(connection) + " -AXtwc 'SELECT text FROM pigsty.inventory;'\n")
	if kind, managed := config.ManagedInventoryScriptShape(body); !managed || kind != config.ManagedInventoryScriptPig {
		return nil, failure("CMDB_CONNECTION_INVALID", "selected connection cannot be rendered as the managed query adapter")
	}
	return body, nil
}

func inspectManagedInventoryScript(plan *PigstySourcePlan) error {
	if plan == nil || plan.InventoryPath == "" || len(plan.scriptBody) == 0 {
		return failure("CMDB_SWITCH_INVALID", "Inventory source plan is incomplete")
	}
	info, err := os.Lstat(plan.InventoryPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		plan.ScriptChanged = true
	case err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
		return failure("CMDB_INVENTORY_SCRIPT_UNMANAGED", "inventory.sh is not an owned regular file")
	default:
		ownerUID, ownerGID, owned := fileOwner(info)
		if !owned || ownerUID != uint32(os.Geteuid()) || ownerGID != uint32(os.Getegid()) {
			return failure("CMDB_SWITCH_PERMISSION", "inventory.sh is not owner-controlled")
		}
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return failure("CMDB_INVENTORY_SCRIPT_UNMANAGED", "inventory.sh has unsupported special mode bits")
		}
		before, readErr := os.ReadFile(plan.InventoryPath)
		if readErr != nil {
			return failure("CMDB_INVENTORY_SCRIPT_UNMANAGED", "inventory.sh could not be inspected")
		}
		if _, managed := config.ManagedInventoryScriptShape(before); !managed {
			return failure("CMDB_INVENTORY_SCRIPT_UNMANAGED", "inventory.sh does not match a supported managed shape")
		}
		plan.inventoryExists = true
		plan.inventoryBefore = bytes.Clone(before)
		plan.inventoryMode = info.Mode()
		plan.inventoryOwnerUID = ownerUID
		plan.inventoryOwnerGID = ownerGID
		plan.ScriptChanged = !bytes.Equal(before, plan.scriptBody) || !exactManagedScriptMode(info.Mode())
	}
	return nil
}

// DisablePigstyInventory mirrors inventory_conf without touching the database
// or deleting inventory.sh.
func DisablePigstyInventory(home string) (PigstySourceResult, error) {
	plan, err := PlanDisablePigstyInventory(home)
	if err != nil {
		return PigstySourceResult{}, err
	}
	return plan.Apply()
}

// PlanDisablePigstyInventory validates the exact supported directive and
// static target without changing local files.
func PlanDisablePigstyInventory(home string) (*PigstySourcePlan, error) {
	home, err := normalizedPigstyHome(home)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, "ansible.cfg")
	inventoryPath := filepath.Join(home, "pigsty.yml")
	info, err := os.Lstat(inventoryPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
		return nil, failure("CMDB_SOURCE_INVALID", "pigsty.yml is not one static regular Inventory")
	}
	ownerUID, ownerGID, owned := fileOwner(info)
	if !owned || ownerUID != uint32(os.Geteuid()) || ownerGID != uint32(os.Getegid()) {
		return nil, failure("CMDB_SWITCH_PERMISSION", "pigsty.yml is not owner-controlled")
	}
	inventoryBefore, err := os.ReadFile(inventoryPath)
	if err != nil {
		return nil, failure("CMDB_SOURCE_INVALID", "pigsty.yml could not be inspected")
	}
	document, err := readAnsibleConfig(configPath)
	if err != nil {
		return nil, err
	}
	patch, changed, err := planGuardedInventoryLiteral(document, home, "pigsty.yml")
	if err != nil {
		return nil, err
	}
	return &PigstySourcePlan{
		ConfigPath: configPath, InventoryPath: inventoryPath,
		Changed: changed, ConfigChanged: changed, configPatch: patch,
		inventoryBefore: bytes.Clone(inventoryBefore), inventoryMode: info.Mode(),
		inventoryOwnerUID: ownerUID, inventoryOwnerGID: ownerGID, inventoryExists: true,
	}, nil
}

func normalizedPigstyHome(home string) (string, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return "", failure("CMDB_SOURCE_INVALID", "Pigsty home is invalid")
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return "", failure("CMDB_SOURCE_INVALID", "Pigsty home is invalid")
	}
	home = filepath.Clean(home)
	info, statErr := os.Lstat(home)
	if statErr != nil || !info.IsDir() || !pathIsSymlinkFree(home) {
		return "", failure("CMDB_SOURCE_INVALID", "Pigsty home must be a symlink-free directory")
	}
	ownerUID, ownerGID, owned := fileOwner(info)
	if !owned || ownerUID != uint32(os.Geteuid()) || ownerGID != uint32(os.Getegid()) {
		return "", failure("CMDB_SOURCE_INVALID", "Pigsty home must be owner-controlled")
	}
	return home, nil
}

func planGuardedInventoryLiteral(document *ansibleConfigDocument, home, targetValue string) (*ansibleConfigPatch, bool, error) {
	if document == nil || (targetValue != "pigsty.yml" && targetValue != "inventory.sh") {
		return nil, false, failure("CMDB_SWITCH_INVALID", "Inventory source plan is invalid")
	}
	if document.ownerUID != uint32(os.Geteuid()) || document.ownerGID != uint32(os.Getegid()) {
		return nil, false, failure("CMDB_SWITCH_PERMISSION", "ansible.cfg is not owner-controlled")
	}
	if document.value != "pigsty.yml" && document.value != "inventory.sh" {
		return nil, false, failure("CMDB_INVENTORY_DIRECTIVE_UNMANAGED", "ansible.cfg Inventory directive is user-owned")
	}
	currentTarget := filepath.Join(home, document.value)
	if document.locator != currentTarget {
		return nil, false, failure("CMDB_INVENTORY_DIRECTIVE_UNMANAGED", "ansible.cfg Inventory directive is outside Pigsty home")
	}
	target := filepath.Join(home, targetValue)
	if document.value == targetValue {
		return &ansibleConfigPatch{document: document, candidate: bytes.Clone(document.body)}, false, nil
	}
	patch, err := planAnsibleConfigLiteralPatch(document, targetValue, target)
	if err != nil {
		return nil, false, err
	}
	return patch, !bytes.Equal(document.body, patch.candidate), nil
}

// Apply commits a previously inspected source plan. It rechecks both local
// source revisions and never overwrites an unrecognized script or directive.
func (p *PigstySourcePlan) Apply() (PigstySourceResult, error) {
	if p == nil || p.ConfigPath == "" || p.InventoryPath == "" || p.configPatch == nil {
		return PigstySourceResult{}, failure("CMDB_SWITCH_INVALID", "Inventory source plan is incomplete")
	}
	current, err := readAnsibleConfig(p.ConfigPath)
	if err != nil || !bytes.Equal(current.body, p.configPatch.document.body) || current.mode != p.configPatch.document.mode ||
		current.ownerUID != p.configPatch.document.ownerUID || current.ownerGID != p.configPatch.document.ownerGID {
		return PigstySourceResult{}, failure("CMDB_SOURCE_CHANGED", "ansible.cfg changed after planning")
	}
	if !p.inventorySourceCurrent() {
		return PigstySourceResult{}, failure("CMDB_SOURCE_CHANGED", "Inventory source changed after planning")
	}
	if p.ScriptChanged {
		if err := utils.AtomicWriteFile(p.InventoryPath, p.scriptBody, 0o755); err != nil {
			return PigstySourceResult{}, failure("CMDB_SWITCH_FAILED", "inventory.sh could not be written")
		}
		if !p.managedCandidateCurrent() {
			return PigstySourceResult{}, failure("CMDB_SWITCH_PARTIAL", "inventory.sh replacement could not be verified")
		}
	}
	if p.ConfigChanged {
		if p.ScriptChanged {
			if !p.managedCandidateCurrent() {
				return PigstySourceResult{}, failure("CMDB_SWITCH_PARTIAL", "inventory.sh changed before ansible.cfg could be switched")
			}
		} else if !p.inventorySourceCurrent() {
			return PigstySourceResult{}, failure("CMDB_SOURCE_CHANGED", "Inventory source changed after planning")
		}
		commit := commitAnsibleConfigPatch
		if p.commitConfig != nil {
			commit = p.commitConfig
		}
		if err := commit(p.configPatch); err != nil {
			if p.ScriptChanged {
				if p.configCandidateCurrent() {
					return PigstySourceResult{}, failure("CMDB_SWITCH_PARTIAL", "ansible.cfg changed but its commit could not be verified")
				}
				if rollbackErr := p.rollbackInventoryScript(); rollbackErr != nil {
					return PigstySourceResult{}, failure("CMDB_SWITCH_PARTIAL", "inventory.sh changed and could not be rolled back after ansible.cfg failed")
				}
			}
			return PigstySourceResult{}, err
		}
	}
	return PigstySourceResult{
		ConfigPath: p.ConfigPath, InventoryPath: p.InventoryPath, Changed: p.Changed,
		ScriptChanged: p.ScriptChanged, ConfigChanged: p.ConfigChanged,
	}, nil
}

func (p *PigstySourcePlan) configCandidateCurrent() bool {
	if p == nil || p.configPatch == nil || p.configPatch.document == nil {
		return false
	}
	current, err := readAnsibleConfig(p.ConfigPath)
	return err == nil && bytes.Equal(current.body, p.configPatch.candidate)
}

func (p *PigstySourcePlan) rollbackInventoryScript() error {
	if p == nil || p.InventoryPath == "" || !p.managedCandidateCurrent() {
		return errors.New("managed Inventory candidate changed before rollback")
	}
	if !p.inventoryExists {
		if err := os.Remove(p.InventoryPath); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(p.InventoryPath)); err != nil {
			return err
		}
		if _, err := os.Lstat(p.InventoryPath); !errors.Is(err, os.ErrNotExist) {
			return errors.New("new Inventory script still exists after rollback")
		}
		return nil
	}
	if err := utils.AtomicWriteFile(p.InventoryPath, p.inventoryBefore, p.inventoryMode.Perm()); err != nil {
		return err
	}
	info, err := os.Lstat(p.InventoryPath)
	if err != nil || info.Mode() != p.inventoryMode {
		return errors.New("Inventory script mode was not restored")
	}
	ownerUID, ownerGID, owned := fileOwner(info)
	if !owned || ownerUID != p.inventoryOwnerUID || ownerGID != p.inventoryOwnerGID {
		return errors.New("Inventory script ownership was not restored")
	}
	body, err := os.ReadFile(p.InventoryPath)
	if err != nil || !bytes.Equal(body, p.inventoryBefore) {
		return errors.New("Inventory script content was not restored")
	}
	return nil
}

func (p *PigstySourcePlan) inventorySourceCurrent() bool {
	if p == nil || p.InventoryPath == "" {
		return false
	}
	info, err := os.Lstat(p.InventoryPath)
	if !p.inventoryExists {
		return errors.Is(err, os.ErrNotExist)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode() != p.inventoryMode {
		return false
	}
	ownerUID, ownerGID, owned := fileOwner(info)
	if !owned || ownerUID != p.inventoryOwnerUID || ownerGID != p.inventoryOwnerGID {
		return false
	}
	body, err := os.ReadFile(p.InventoryPath)
	return err == nil && bytes.Equal(body, p.inventoryBefore)
}

func (p *PigstySourcePlan) managedCandidateCurrent() bool {
	if p == nil || p.InventoryPath == "" || len(p.scriptBody) == 0 {
		return false
	}
	info, err := os.Lstat(p.InventoryPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !exactManagedScriptMode(info.Mode()) {
		return false
	}
	ownerUID, ownerGID, owned := fileOwner(info)
	if !owned || ownerUID != uint32(os.Geteuid()) || ownerGID != uint32(os.Getegid()) {
		return false
	}
	body, err := os.ReadFile(p.InventoryPath)
	return err == nil && bytes.Equal(body, p.scriptBody)
}

func exactManagedScriptMode(mode os.FileMode) bool {
	return mode.Perm() == 0o755 && mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0
}

func literalConnectionHasPassword(value string) bool {
	return config.InventoryConnectionHasPassword(value)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
