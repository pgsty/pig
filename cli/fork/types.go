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
	SourceDB     string
	DestDB       string
	Owner        string
	ConnDB       string
	Port         int
	ConnLimit    int
	ConnLimitSet bool
	Kill         bool
	Preflight    DatabasePreflight
	Warnings     []string
	OwnerChanged bool
	OwnerWarning string
}

type State struct {
	BackupMode BackupMode
	CloneMode  CloneMode
	FS         string
	Started    bool
}

type ResultData struct {
	Kind            Kind               `json:"kind" yaml:"kind"`
	Source          string             `json:"source" yaml:"source"`
	Destination     string             `json:"destination" yaml:"destination"`
	SourcePort      int                `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	DestinationPort int                `json:"destination_port,omitempty" yaml:"destination_port,omitempty"`
	BackupMode      string             `json:"backup_mode,omitempty" yaml:"backup_mode,omitempty"`
	CloneMode       string             `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Started         bool               `json:"started" yaml:"started"`
	ConnectCommand  string             `json:"connect_command,omitempty" yaml:"connect_command,omitempty"`
	CleanupCommand  string             `json:"cleanup_command,omitempty" yaml:"cleanup_command,omitempty"`
	Duration        float64            `json:"duration_seconds" yaml:"duration_seconds"`
	Preflight       *DatabasePreflight `json:"preflight,omitempty" yaml:"preflight,omitempty"`
	Warnings        []string           `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	OwnerRequested  string             `json:"owner_requested,omitempty" yaml:"owner_requested,omitempty"`
	OwnerChanged    bool               `json:"owner_changed,omitempty" yaml:"owner_changed,omitempty"`
	OwnerWarning    string             `json:"owner_warning,omitempty" yaml:"owner_warning,omitempty"`
}

type DatabasePreflight struct {
	ServerVersion       int       `json:"server_version" yaml:"server_version"`
	FileCopyMethod      string    `json:"file_copy_method,omitempty" yaml:"file_copy_method,omitempty"`
	FileCopyMethodError string    `json:"file_copy_method_error,omitempty" yaml:"file_copy_method_error,omitempty"`
	DataDirectory       string    `json:"data_directory,omitempty" yaml:"data_directory,omitempty"`
	FileSystem          string    `json:"file_system,omitempty" yaml:"file_system,omitempty"`
	CloneMode           CloneMode `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Strategy            string    `json:"strategy" yaml:"strategy"`
	FileSystemError     string    `json:"file_system_error,omitempty" yaml:"file_system_error,omitempty"`
}

func (p DatabasePreflight) Warnings() []string {
	warnings := []string{}
	if p.ServerVersion > 0 && p.ServerVersion < 180000 {
		warnings = append(warnings, fmt.Sprintf("PostgreSQL 18+ is recommended for CoW database clone, current server_version_num=%d", p.ServerVersion))
	}
	if p.ServerVersion == 0 {
		warnings = append(warnings, "PostgreSQL version could not be verified")
	}
	if !strings.EqualFold(p.FileCopyMethod, "clone") {
		if p.FileCopyMethod == "" {
			if p.FileCopyMethodError != "" {
				warnings = append(warnings, fmt.Sprintf("file_copy_method=clone could not be verified: %s", p.FileCopyMethodError))
			} else {
				warnings = append(warnings, "file_copy_method=clone could not be verified")
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("file_copy_method=clone is recommended, current value is %s", p.FileCopyMethod))
		}
	}
	if p.CloneMode != CloneModeCOW {
		if p.FileSystemError != "" {
			warnings = append(warnings, fmt.Sprintf("CoW clone support could not be verified for data_directory %s: %s", p.DataDirectory, p.FileSystemError))
		} else {
			warnings = append(warnings, fmt.Sprintf("CoW clone is not confirmed for data_directory %s on filesystem %s", p.DataDirectory, p.FileSystem))
		}
	}
	return warnings
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
	db.SourceDB = strings.TrimSpace(db.SourceDB)
	db.DestDB = strings.TrimSpace(db.DestDB)
	db.Owner = strings.TrimSpace(db.Owner)
	db.ConnDB = strings.TrimSpace(db.ConnDB)
	if db.SourceDB == "" {
		return fmt.Errorf("source database is required")
	}
	if strings.EqualFold(db.SourceDB, "template0") || strings.EqualFold(db.SourceDB, "template1") {
		return fmt.Errorf("source database %q is a system template; clone an existing user database instead", db.SourceDB)
	}
	if db.ConnDB == "" {
		if strings.EqualFold(db.SourceDB, "postgres") {
			db.ConnDB = "template1"
		} else {
			db.ConnDB = "postgres"
		}
	}
	if strings.EqualFold(db.ConnDB, db.SourceDB) {
		return fmt.Errorf("connection database must differ from source database %q", db.SourceDB)
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
	if db.ConnLimitSet && db.ConnLimit < -1 {
		return fmt.Errorf("invalid connection limit %d (must be -1 or greater)", db.ConnLimit)
	}
	db.Kill = true
	return nil
}
