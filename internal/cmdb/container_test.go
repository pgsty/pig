package cmdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	containerTestFlag = "PIGSTY_CMDB_CONTAINER_TEST"
	postgres18Image   = "postgres:18.4-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
)

var cmdbContainer *containerDatabase

type containerDatabase struct {
	name             string
	database         string
	dsn              string
	selectedPlatform string
	cleanupOnce      sync.Once
}

func TestMain(m *testing.M) {
	if os.Getenv(containerTestFlag) != "1" {
		os.Exit(m.Run())
	}
	harness, err := startContainerDatabase(postgres18Image)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CMDB container setup failed:", err)
		os.Exit(1)
	}
	cmdbContainer = harness
	code := m.Run()
	harness.cleanup()
	os.Exit(code)
}

func startContainerDatabase(image string) (*containerDatabase, error) {
	if image != postgres18Image {
		return nil, fmt.Errorf("unreviewed PostgreSQL test image")
	}
	suffix, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	database := "pig_sty_cmdb_test_" + suffix
	name := "pig-sty-cmdb-test-" + suffix
	password := "test-" + suffix

	inspect := exec.Command("docker", "image", "inspect", image,
		"--format", "{{index .RepoDigests 0}}|{{.Os}}/{{.Architecture}}|{{.Id}}")
	identity, err := inspect.Output()
	if err != nil {
		return nil, fmt.Errorf("pinned image is unavailable locally")
	}
	parts := strings.Split(strings.TrimSpace(string(identity)), "|")
	if len(parts) != 3 || !strings.Contains(parts[0], "@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15") {
		return nil, fmt.Errorf("pinned image identity mismatch")
	}

	run := exec.Command(
		"docker", "run", "--detach", "--rm",
		"--name", name,
		"--publish", "127.0.0.1::5432",
		"--env", "POSTGRES_PASSWORD="+password,
		"--env", "POSTGRES_DB="+database,
		image,
	)
	if output, err := run.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("container start failed: %s", strings.TrimSpace(string(output)))
	}
	platformInspect := exec.Command("docker", "image", "inspect", "--platform", parts[1], image,
		"--format", "{{.Descriptor.digest}}")
	platformDigest, err := platformInspect.Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(platformDigest)), "sha256:") {
		return nil, fmt.Errorf("selected platform image identity is unavailable")
	}
	harness := &containerDatabase{
		name:             name,
		database:         database,
		selectedPlatform: parts[1] + "@" + strings.TrimSpace(string(platformDigest)),
	}
	fail := func(cause error) (*containerDatabase, error) {
		harness.cleanup()
		return nil, cause
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		ready := exec.Command("docker", "exec", name, "pg_isready", "-U", "postgres", "-d", database)
		if ready.Run() == nil {
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("PostgreSQL readiness timeout"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	portOutput, err := exec.Command("docker", "port", name, "5432/tcp").Output()
	if err != nil {
		return fail(fmt.Errorf("container port lookup failed"))
	}
	host, portText, err := net.SplitHostPort(strings.TrimSpace(string(portOutput)))
	if err != nil || !isLoopbackHost(host) {
		return fail(fmt.Errorf("container is not bound to loopback"))
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fail(fmt.Errorf("container port is invalid"))
	}
	harness.dsn = (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("postgres", password),
		Host:     net.JoinHostPort(host, portText),
		Path:     "/" + database,
		RawQuery: "sslmode=disable",
	}).String()
	if err := harness.guard(); err != nil {
		return fail(err)
	}
	for {
		connection, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, portText), time.Second)
		if dialErr == nil {
			_ = connection.Close()
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("published PostgreSQL port readiness timeout"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	for {
		connectCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		connection, connectErr := pgx.Connect(connectCtx, harness.dsn)
		cancel()
		if connectErr == nil {
			_ = connection.Close(context.Background())
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("published PostgreSQL connection readiness timeout"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	return harness, nil
}

func (c *containerDatabase) guard() error {
	parsed, err := url.Parse(c.dsn)
	if err != nil {
		return fmt.Errorf("test DSN is invalid")
	}
	host := parsed.Hostname()
	if !isLoopbackHost(host) || !strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "pig_sty_cmdb_test_") ||
		strings.TrimPrefix(parsed.Path, "/") != c.database {
		return fmt.Errorf("destructive test guard rejected database target")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func (c *containerDatabase) cleanup() {
	if c == nil || c.name == "" {
		return
	}
	c.cleanupOnce.Do(func() {
		_ = exec.Command("docker", "rm", "--force", c.name).Run()
	})
}

func openContainerStore(t *testing.T) *Store {
	t.Helper()
	if cmdbContainer == nil {
		t.Skip("set " + containerTestFlag + "=1 to run pinned PostgreSQL tests")
	}
	if err := cmdbContainer.guard(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), ResolvedConnection{
		source: ConnectionEnvSource,
		value:  cmdbContainer.dsn,
	}, OpenOptions{Timeouts: Timeouts{Transaction: 15 * time.Second}})
	if err != nil {
		config, parseErr := pgx.ParseConfig(cmdbContainer.dsn)
		if parseErr != nil {
			t.Fatalf("%v; direct parse stage failed (%T)", err, parseErr)
		}
		connection, connectErr := pgx.ConnectConfig(t.Context(), config)
		if connectErr != nil {
			t.Fatalf("%v; direct connect stage failed (%T)", err, connectErr)
		}
		defer connection.Close(t.Context())
		var database, address string
		var oid uint32
		var port int32
		var version int
		var boot time.Time
		scanErr := connection.QueryRow(t.Context(), `
SELECT current_database(), d.oid, COALESCE(inet_server_addr()::text, 'local'),
       inet_server_port(), current_setting('server_version_num')::integer,
       pg_postmaster_start_time()
  FROM pg_database AS d
 WHERE d.datname = current_database()`).Scan(&database, &oid, &address, &port, &version, &boot)
		t.Fatalf("%v; direct identity stage failed (%T)", err, scanErr)
	}
	t.Cleanup(store.Close)
	return store
}

func resetContainerSchema(t *testing.T, store *Store) {
	t.Helper()
	if cmdbContainer == nil || cmdbContainer.guard() != nil {
		t.Fatal("destructive test guard failed")
	}
	if _, err := store.pool.Exec(t.Context(), `DROP SCHEMA IF EXISTS pigsty_cmdb CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
