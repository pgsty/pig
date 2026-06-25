/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

Package fork provides instance-level and database-level PostgreSQL fork support.
*/
package fork

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"pig/internal/utils"
)

type Kind string

const (
	KindInstance Kind = "instance"
	KindDatabase Kind = "database"
)

type Mode string

const (
	ModeAuto Mode = "auto"
	ModeHot  Mode = "hot"
	ModeCold Mode = "cold"
)

type BackupMode string

const (
	BackupModeUnknown BackupMode = "unknown"
	BackupModeHot     BackupMode = "hot"
	BackupModeCold    BackupMode = "cold"
)

type CloneMode string

const (
	CloneModeUnknown CloneMode = "unknown"
	CloneModeCOW     CloneMode = "cow"
	CloneModeCopy    CloneMode = "copy"
)

type Options struct {
	Kind     Kind
	Mode     Mode
	DbSU     string
	Plan     bool
	Yes      bool
	Run      bool
	Start    bool
	NoStart  bool
	Replace  bool
	Instance InstanceOptions
	Database DatabaseOptions
}

type InstanceOptions struct {
	Name       string
	SourceData string
	SourcePort int
	DestData   string
	DestPort   int
	Timeout    int
}

type DatabaseOptions struct {
	SourceDB string
	DestDB   string
	ConnDB   string
	Port     int
	Kill     bool
	NoKill   bool
	Strategy string
}

type State struct {
	BackupMode BackupMode
	CloneMode  CloneMode
	FS         string
	Started    bool
}

type ResultData struct {
	Kind            Kind    `json:"kind" yaml:"kind"`
	Source          string  `json:"source" yaml:"source"`
	Destination     string  `json:"destination" yaml:"destination"`
	SourcePort      int     `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	DestinationPort int     `json:"destination_port,omitempty" yaml:"destination_port,omitempty"`
	BackupMode      string  `json:"backup_mode,omitempty" yaml:"backup_mode,omitempty"`
	CloneMode       string  `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Started         bool    `json:"started" yaml:"started"`
	ConnectCommand  string  `json:"connect_command,omitempty" yaml:"connect_command,omitempty"`
	CleanupCommand  string  `json:"cleanup_command,omitempty" yaml:"cleanup_command,omitempty"`
	Duration        float64 `json:"duration_seconds" yaml:"duration_seconds"`
}

var forkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func NormalizeOptions(opts *Options) (*Options, error) {
	if opts == nil {
		return nil, fmt.Errorf("fork options are required")
	}
	n := *opts
	n.DbSU = utils.GetDBSU(n.DbSU)
	if n.Mode == "" {
		n.Mode = ModeAuto
	}
	if n.Mode != ModeAuto && n.Mode != ModeHot && n.Mode != ModeCold {
		return nil, fmt.Errorf("invalid fork mode %q (valid: auto, hot, cold)", n.Mode)
	}
	n.Start = n.Run && !n.NoStart

	switch n.Kind {
	case KindInstance:
		if err := normalizeInstance(&n); err != nil {
			return nil, err
		}
	case KindDatabase:
		if err := normalizeDatabase(&n); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid fork kind %q (valid: instance, database)", n.Kind)
	}
	return &n, nil
}

func normalizeInstance(opts *Options) error {
	inst := &opts.Instance
	inst.Name = strings.TrimSpace(inst.Name)
	if inst.Name == "" {
		return fmt.Errorf("fork name is required")
	}
	if !forkNamePattern.MatchString(inst.Name) {
		return fmt.Errorf("invalid fork name %q (use letters, numbers, dot, underscore, or dash)", inst.Name)
	}
	if inst.SourceData == "" {
		inst.SourceData = os.Getenv("PG_DATA")
	}
	if inst.SourceData == "" {
		inst.SourceData = "/pg/data"
	}
	if inst.SourcePort == 0 {
		if port := os.Getenv("PG_PORT"); port != "" {
			if p, err := strconv.Atoi(port); err == nil && p > 0 {
				inst.SourcePort = p
			}
		}
	}
	if inst.SourcePort == 0 {
		inst.SourcePort = 5432
	}
	if !validPort(inst.SourcePort) {
		return fmt.Errorf("invalid source port %d (must be 1-65535)", inst.SourcePort)
	}
	if inst.DestData == "" {
		inst.DestData = "/pg/data-" + inst.Name
	}
	if inst.DestPort == 0 {
		inst.DestPort = firstFreePort(15432)
	}
	if !validPort(inst.DestPort) {
		return fmt.Errorf("invalid destination port %d (must be 1-65535)", inst.DestPort)
	}
	if inst.Timeout == 0 {
		inst.Timeout = 60
	}
	return nil
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func firstFreePort(start int) int {
	for port := start; port < start+1000; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port
	}
	return start
}

func normalizeDatabase(opts *Options) error {
	db := &opts.Database
	if db.SourceDB == "" {
		return fmt.Errorf("source database is required")
	}
	if db.DestDB == "" {
		return fmt.Errorf("destination database is required")
	}
	if db.ConnDB == "" {
		db.ConnDB = "postgres"
	}
	if db.Port == 0 {
		if port := os.Getenv("PG_PORT"); port != "" {
			if p, err := strconv.Atoi(port); err == nil && p > 0 {
				db.Port = p
			}
		}
	}
	if db.Port == 0 {
		db.Port = 5432
	}
	if db.Strategy == "" {
		db.Strategy = "FILE_COPY"
	}
	db.Strategy = strings.ToUpper(strings.ReplaceAll(db.Strategy, "-", "_"))
	if db.Strategy != "FILE_COPY" && db.Strategy != "WAL_LOG" {
		return fmt.Errorf("invalid database clone strategy %q (valid: FILE_COPY, WAL_LOG)", db.Strategy)
	}
	db.Kill = shouldKillDatabaseConnections(*db)
	return nil
}

func shouldKillDatabaseConnections(db DatabaseOptions) bool {
	if db.NoKill {
		return false
	}
	sourceDB := strings.TrimSpace(db.SourceDB)
	if sourceDB == "" {
		return false
	}
	return !strings.EqualFold(sourceDB, "template0") && !strings.EqualFold(sourceDB, "template1")
}
