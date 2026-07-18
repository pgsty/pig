package cmdb

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func (c ResolvedConnection) Source() ConnectionSource { return c.source }

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

// OpenOptions controls the small native pool. Zero values use safe defaults.
type OpenOptions struct {
	Timeouts Timeouts
	MaxConns int32
}

type targetIdentity struct {
	Database       string
	DatabaseOID    uint32
	ServerAddress  string
	ServerPort     int32
	ServerVersion  int
	PostmasterBoot time.Time
}

// Store owns a bounded pgx pool pinned to one database/server identity.
type Store struct {
	pool       *pgxpool.Pool
	source     ConnectionSource
	connection string
	timeouts   Timeouts
	identityMu sync.Mutex
	identity   *targetIdentity
}

// Open connects eagerly, applies bounded runtime settings, and pins every pool
// connection to the first observed target identity.
func Open(ctx context.Context, resolved ResolvedConnection, options OpenOptions) (*Store, error) {
	if resolved.value == "" {
		return nil, failure("CMDB_CONNECTION_INVALID", "connection source is unresolved")
	}
	timeouts := normalizeTimeouts(options.Timeouts)
	config, err := pgxpool.ParseConfig(resolved.value)
	if err != nil {
		return nil, failure("CMDB_CONNECTION_INVALID", "connection configuration is invalid")
	}
	maxConns := options.MaxConns
	if maxConns <= 0 {
		maxConns = 4
	} else if maxConns > 8 {
		maxConns = 8
	}
	config.MaxConns = maxConns
	config.MinConns = 0
	config.MinIdleConns = 0
	config.MaxConnIdleTime = time.Minute
	config.MaxConnLifetime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second
	config.PingTimeout = timeouts.Connect
	config.ConnConfig.ConnectTimeout = timeouts.Connect
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["application_name"] = "pig-inventory-cmdb"
	config.ConnConfig.RuntimeParams["statement_timeout"] = durationMilliseconds(timeouts.Statement)
	config.ConnConfig.RuntimeParams["lock_timeout"] = durationMilliseconds(timeouts.Lock)
	config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = durationMilliseconds(timeouts.Transaction)

	store := &Store{source: resolved.source, connection: resolved.value, timeouts: timeouts}
	config.AfterConnect = store.verifyConnectionIdentity
	connectCtx, cancel := context.WithTimeout(ctx, timeouts.Connect)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, config)
	if err != nil {
		return nil, failure("CMDB_CONNECTION_FAILED", "database connection failed")
	}
	store.pool = pool
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		if IsCode(err, "CMDB_TARGET_CHANGED") {
			return nil, err
		}
		return nil, failure("CMDB_CONNECTION_FAILED", "database connection failed")
	}
	return store, nil
}

func (s *Store) verifyConnectionIdentity(ctx context.Context, connection *pgx.Conn) error {
	var identity targetIdentity
	err := connection.QueryRow(ctx, `
SELECT current_database(), d.oid, COALESCE(inet_server_addr()::text, 'local'),
       COALESCE(inet_server_port(), 0), current_setting('server_version_num')::integer,
       pg_postmaster_start_time()
  FROM pg_database AS d
 WHERE d.datname = current_database()`).Scan(
		&identity.Database,
		&identity.DatabaseOID,
		&identity.ServerAddress,
		&identity.ServerPort,
		&identity.ServerVersion,
		&identity.PostmasterBoot,
	)
	if err != nil {
		return failure("CMDB_CONNECTION_FAILED", "database identity could not be verified")
	}
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	if s.identity == nil {
		copy := identity
		s.identity = &copy
		return nil
	}
	if *s.identity != identity {
		return failure("CMDB_TARGET_CHANGED", "connection pool resolved to a different database target")
	}
	return nil
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
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	if s.identity == nil {
		return ""
	}
	identity := s.identity
	material := fmt.Sprintf("%s\x00%d\x00%s\x00%d\x00%d\x00%d",
		identity.Database, identity.DatabaseOID, identity.ServerAddress, identity.ServerPort,
		identity.ServerVersion, identity.PostmasterBoot.UnixNano())
	sum := sha256.Sum256([]byte(material))
	return fmt.Sprintf("sha256:%x", sum)
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
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
