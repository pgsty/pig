package cmdb

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConnectionSource is the only connection metadata safe for public results.
type ConnectionSource string

const (
	ConnectionMetaSource ConnectionSource = "service:meta"
	ConnectionArgSource  ConnectionSource = "argument"
	ConnectionURLSource  ConnectionSource = "environment:METADB_URL"
)

// ResolvedConnection keeps the connection value private to this package.
// Callers can report Source but cannot recover a password-bearing value.
type ResolvedConnection struct {
	source ConnectionSource
	value  string
}

// Timeouts are bounded again by normalizeTimeouts before driver use.
type Timeouts struct {
	Connect     time.Duration
	Statement   time.Duration
	Lock        time.Duration
	Transaction time.Duration
}

var defaultTimeouts = Timeouts{
	Connect:     5 * time.Second,
	Statement:   15 * time.Second,
	Lock:        3 * time.Second,
	Transaction: 30 * time.Second,
}

// OpenOptions controls the native database connection. Zero values use safe defaults.
type OpenOptions struct {
	Timeouts Timeouts
}

// Store owns one connection pinned to one database/server identity.
type Store struct {
	conn              *pgx.Conn
	source            ConnectionSource
	connection        string
	timeouts          Timeouts
	targetFingerprint string
}

// Open connects eagerly, applies bounded runtime settings, and records the
// connected target identity for confirmation pinning.
func Open(ctx context.Context, resolved ResolvedConnection, options OpenOptions) (*Store, error) {
	if resolved.value == "" {
		return nil, failure("CMDB_CONNECTION_INVALID", "connection source is unresolved")
	}
	timeouts := normalizeTimeouts(options.Timeouts)
	config, err := pgx.ParseConfig(resolved.value)
	if err != nil {
		return nil, failure("CMDB_CONNECTION_INVALID", "connection configuration is invalid")
	}
	config.ConnectTimeout = timeouts.Connect
	if config.RuntimeParams == nil {
		config.RuntimeParams = make(map[string]string)
	}
	config.RuntimeParams["application_name"] = "pig-inventory-cmdb"
	config.RuntimeParams["statement_timeout"] = durationMilliseconds(timeouts.Statement)
	config.RuntimeParams["lock_timeout"] = durationMilliseconds(timeouts.Lock)
	config.RuntimeParams["idle_in_transaction_session_timeout"] = durationMilliseconds(timeouts.Transaction)

	connectCtx, cancel := context.WithTimeout(ctx, timeouts.Connect)
	defer cancel()
	connection, err := pgx.ConnectConfig(connectCtx, config)
	if err != nil {
		return nil, failure("CMDB_CONNECTION_FAILED", "database connection failed")
	}
	fingerprint, err := targetFingerprint(connectCtx, connection)
	if err != nil {
		_ = connection.Close(context.Background())
		return nil, err
	}
	return &Store{
		conn: connection, source: resolved.source, connection: resolved.value,
		timeouts: timeouts, targetFingerprint: fingerprint,
	}, nil
}

func targetFingerprint(ctx context.Context, connection *pgx.Conn) (string, error) {
	var database, address string
	var databaseOID uint32
	var port int32
	var version int
	var postmasterBoot time.Time
	err := connection.QueryRow(ctx, `
SELECT current_database(), d.oid, COALESCE(inet_server_addr()::text, 'local'),
       COALESCE(inet_server_port(), 0), current_setting('server_version_num')::integer,
       pg_postmaster_start_time()
  FROM pg_database AS d
 WHERE d.datname = current_database()`).Scan(
		&database, &databaseOID, &address, &port, &version, &postmasterBoot,
	)
	if err != nil {
		return "", failure("CMDB_CONNECTION_FAILED", "database identity could not be verified")
	}
	material := fmt.Sprintf("%s\x00%d\x00%s\x00%d\x00%d\x00%d",
		database, databaseOID, address, port, version, postmasterBoot.UnixNano())
	sum := sha256.Sum256([]byte(material))
	return fmt.Sprintf("sha256:%x", sum), nil
}

func normalizeTimeouts(value Timeouts) Timeouts {
	return Timeouts{
		Connect:     boundedDuration(value.Connect, defaultTimeouts.Connect, 30*time.Second),
		Statement:   boundedDuration(value.Statement, defaultTimeouts.Statement, 2*time.Minute),
		Lock:        boundedDuration(value.Lock, defaultTimeouts.Lock, 30*time.Second),
		Transaction: boundedDuration(value.Transaction, defaultTimeouts.Transaction, 5*time.Minute),
	}
}

func boundedDuration(value, fallback, maximum time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func durationMilliseconds(value time.Duration) string {
	milliseconds := value.Milliseconds()
	if milliseconds < 1 {
		milliseconds = 1
	}
	return strconv.FormatInt(milliseconds, 10)
}

func (s *Store) Source() ConnectionSource {
	if s == nil {
		return ""
	}
	return s.source
}

// TargetFingerprint returns a one-way identifier for the pinned database
// identity. It is safe for plans and confirmation pinning and reveals no DSN,
// hostname, address, database name, or credential.
func (s *Store) TargetFingerprint() string {
	if s == nil {
		return ""
	}
	return s.targetFingerprint
}

func (s *Store) Close() {
	if s != nil && s.conn != nil {
		_ = s.conn.Close(context.Background())
	}
}

func (s *Store) transactionContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, s.timeouts.Transaction)
}

func sanitizeDatabaseFailure(code, action string, err error) error {
	if err == nil {
		return nil
	}
	return failure(code, fmt.Sprintf("database %s failed", action))
}
