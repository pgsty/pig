/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

PostgreSQL parameter tuning based on hardware detection.
Generates optimized parameters from environment detection.
*/
package postgres

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// ============================================================================
// Options & Data Structures
// ============================================================================

// TuneOptions holds user-specified flags for the tune command.
type TuneOptions struct {
	Profile    string  // oltp, olap, tiny, crit
	CPU        int     // 0 = auto-detect
	MemMB      int     // 0 = auto-detect
	DiskGB     int     // 0 = auto-detect
	MaxConn    int     // 0 = default (100)
	ShmemRatio float64 // default 0.25
}

// TuneSpec holds detected/resolved hardware specs.
type TuneSpec struct {
	CPU    int // cores
	MemMB  int // total memory in MB
	DiskGB int // data disk size in GB
}

// TuneParam represents a single tuned parameter.
type TuneParam struct {
	Name    string // parameter name (lowercase)
	Value   string // value with unit, e.g. "8192MB"
	Section string // grouping: connection, memory, cpu, storage, disk
	PgMin   int    // minimum PG version required (0 = all)
}

// ============================================================================
// Profile Definitions
// ============================================================================

type tuneProfile struct {
	Name                 string
	MaxConn              int
	MaintenanceMemFactor float64
	WorkMemMin           int
	WorkMemMax           int
	WorkerProcessesBase  int
	WorkerProcessesMin   int
	ParallelWorkerRatio  float64
	ParallelWorkerMin    int
	ParallelGatherRatio  float64
	ParallelGatherMin    int
	ParallelGatherMax    int // -1 = no upper bound
	ParallelGatherFixed  int // >= 0: fixed value; -1: use formula
	ParallelMaintRatio   float64
	ParallelMaintMin     int
	IOWorkerRatio        float64
	IOWorkerMin          int
	IOWorkerMax          int
	IOWorkerFixed        int // >= 0: fixed value; -1: use formula
	TempFileFactor       int
	TempFileCap          int
	CeilParallel         bool // true = ceil for parallel workers; false = floor (tiny)
}

var tuneProfiles = map[string]tuneProfile{
	"oltp": {
		Name: "oltp", MaxConn: 100, CeilParallel: true,
		MaintenanceMemFactor: 0.25, WorkMemMin: 64, WorkMemMax: 1024,
		WorkerProcessesBase: 8, WorkerProcessesMin: 16,
		ParallelWorkerRatio: 0.5, ParallelWorkerMin: 2,
		ParallelGatherRatio: 0.2, ParallelGatherMin: 2, ParallelGatherMax: 8, ParallelGatherFixed: -1,
		ParallelMaintRatio: 0.33, ParallelMaintMin: 2,
		IOWorkerRatio: 0.25, IOWorkerMin: 4, IOWorkerMax: 16, IOWorkerFixed: -1,
		TempFileFactor: 1, TempFileCap: 200,
	},
	"olap": {
		Name: "olap", MaxConn: 100, CeilParallel: true,
		MaintenanceMemFactor: 0.50, WorkMemMin: 64, WorkMemMax: 8192,
		WorkerProcessesBase: 12, WorkerProcessesMin: 20,
		ParallelWorkerRatio: 0.8, ParallelWorkerMin: 2,
		ParallelGatherRatio: 0.5, ParallelGatherMin: 2, ParallelGatherMax: -1, ParallelGatherFixed: -1,
		ParallelMaintRatio: 0.33, ParallelMaintMin: 2,
		IOWorkerRatio: 0.5, IOWorkerMin: 4, IOWorkerMax: 32, IOWorkerFixed: -1,
		TempFileFactor: 4, TempFileCap: 2000,
	},
	"tiny": {
		Name: "tiny", MaxConn: 100, CeilParallel: false,
		MaintenanceMemFactor: 0.25, WorkMemMin: 16, WorkMemMax: 256,
		WorkerProcessesBase: 4, WorkerProcessesMin: 12,
		ParallelWorkerRatio: 0.5, ParallelWorkerMin: 1,
		ParallelGatherRatio: 0, ParallelGatherMin: 0, ParallelGatherMax: 0, ParallelGatherFixed: 0,
		ParallelMaintRatio: 0.33, ParallelMaintMin: 1,
		IOWorkerRatio: 0, IOWorkerMin: 3, IOWorkerMax: 3, IOWorkerFixed: 3,
		TempFileFactor: 1, TempFileCap: 200,
	},
	"crit": {
		Name: "crit", MaxConn: 100, CeilParallel: true,
		MaintenanceMemFactor: 0.25, WorkMemMin: 64, WorkMemMax: 1024,
		WorkerProcessesBase: 8, WorkerProcessesMin: 16,
		ParallelWorkerRatio: 0.5, ParallelWorkerMin: 2,
		ParallelGatherRatio: 0, ParallelGatherMin: 0, ParallelGatherMax: 0, ParallelGatherFixed: 0,
		ParallelMaintRatio: 0.33, ParallelMaintMin: 2,
		IOWorkerRatio: 0.25, IOWorkerMin: 4, IOWorkerMax: 8, IOWorkerFixed: -1,
		TempFileFactor: 1, TempFileCap: 200,
	},
}

// ============================================================================
// Core Calculation
// ============================================================================

// CalculateTuneParams computes all tuned parameters from hardware spec and profile.
func CalculateTuneParams(spec TuneSpec, prof tuneProfile, maxConn int,
	shmemRatio float64, pgVersion int) []TuneParam {

	var params []TuneParam
	conn := maxConn
	if conn <= 0 {
		conn = prof.MaxConn
	}

	// Connection
	params = append(params, TuneParam{"max_connections", itoa(conn), "connection", 0})

	// Memory
	sb := ceilInt(float64(spec.MemMB) * shmemRatio)
	maintMem := ceilInt(float64(sb) * prof.MaintenanceMemFactor)
	workMem := clampOpt(floorInt(float64(sb)/float64(conn)), prof.WorkMemMin, prof.WorkMemMax)
	effCache := spec.MemMB - sb

	params = append(params,
		TuneParam{"shared_buffers", mb(sb), "memory", 0},
		TuneParam{"maintenance_work_mem", mb(maintMem), "memory", 0},
		TuneParam{"work_mem", mb(workMem), "memory", 0},
		TuneParam{"effective_cache_size", mb(effCache), "memory", 0},
		TuneParam{"huge_pages", "try", "memory", 0},
	)
	if pgVersion >= 13 {
		params = append(params, TuneParam{"hash_mem_multiplier", "8.0", "memory", 13})
	}

	// CPU / Parallel
	cpu := spec.CPU
	useCeil := prof.CeilParallel

	mwp := maxInt(cpu+prof.WorkerProcessesBase, prof.WorkerProcessesMin)
	mpw := maxInt(roundParallel(float64(cpu)*prof.ParallelWorkerRatio, useCeil), prof.ParallelWorkerMin)

	var mpwg int
	if prof.ParallelGatherFixed >= 0 {
		mpwg = prof.ParallelGatherFixed
	} else {
		mpwg = clampOpt(floorInt(float64(cpu)*prof.ParallelGatherRatio), prof.ParallelGatherMin, prof.ParallelGatherMax)
	}

	mpmw := maxInt(roundParallel(float64(cpu)*prof.ParallelMaintRatio, useCeil), prof.ParallelMaintMin)

	params = append(params,
		TuneParam{"max_worker_processes", itoa(mwp), "cpu", 0},
		TuneParam{"max_parallel_workers", itoa(mpw), "cpu", 0},
		TuneParam{"max_parallel_workers_per_gather", itoa(mpwg), "cpu", 0},
		TuneParam{"max_parallel_maintenance_workers", itoa(mpmw), "cpu", 0},
	)

	if pgVersion >= 18 {
		var iow int
		if prof.IOWorkerFixed >= 0 {
			iow = prof.IOWorkerFixed
		} else {
			iow = clampOpt(ceilInt(float64(cpu)*prof.IOWorkerRatio), prof.IOWorkerMin, prof.IOWorkerMax)
		}
		params = append(params, TuneParam{"io_workers", itoa(iow), "cpu", 18})
	}

	// Storage (always SSD)
	params = append(params,
		TuneParam{"random_page_cost", "1.1", "storage", 0},
		TuneParam{"effective_io_concurrency", "200", "storage", 0},
	)
	if pgVersion >= 13 {
		params = append(params, TuneParam{"maintenance_io_concurrency", "100", "storage", 13})
	}

	// Disk / WAL
	sizeTwentieth := clampOpt(ceilInt(float64(spec.DiskGB)/20.0), 1, 100)
	params = append(params,
		TuneParam{"min_wal_size", gb(minInt(sizeTwentieth, 200)), "disk", 0},
		TuneParam{"max_wal_size", gb(minInt(sizeTwentieth*4, 2000)), "disk", 0},
		TuneParam{"max_slot_wal_keep_size", gb(minInt(sizeTwentieth*6, 3000)), "disk", 0},
		TuneParam{"temp_file_limit", gb(minInt(sizeTwentieth*prof.TempFileFactor, prof.TempFileCap)), "disk", 0},
	)

	return params
}

// ============================================================================
// Hardware Detection
// ============================================================================

// DetectHardware resolves hardware specs, using user overrides where provided.
func DetectHardware(cfg *Config, opts *TuneOptions) TuneSpec {
	var spec TuneSpec

	if opts.CPU > 0 {
		spec.CPU = opts.CPU
	} else {
		spec.CPU = runtime.NumCPU()
		if spec.CPU < 1 {
			spec.CPU = 1
		}
	}

	if opts.MemMB > 0 {
		spec.MemMB = opts.MemMB
	} else {
		spec.MemMB = detectMemoryMB()
		if spec.MemMB < 256 {
			logrus.Warnf("memory detection returned %dMB, using fallback 1024MB", spec.MemMB)
			spec.MemMB = 1024
		}
	}

	if opts.DiskGB > 0 {
		spec.DiskGB = opts.DiskGB
	} else {
		dataDir := GetPgData(cfg)
		spec.DiskGB = detectDiskGB(dataDir)
		if spec.DiskGB < 1 {
			logrus.Warnf("disk detection returned %dGB for %s, using fallback 40GB", spec.DiskGB, dataDir)
			spec.DiskGB = 40
		}
	}

	return spec
}

// detectMemoryMB reads total memory from /proc/meminfo.
// Returns 0 on failure.
func detectMemoryMB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		logrus.Debugf("cannot open /proc/meminfo: %v", err)
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

// detectDiskGB runs df on the given path and returns total disk size in GB.
// Returns 0 on failure.
func detectDiskGB(path string) int {
	out, err := exec.Command("df", "-k", path).Output()
	if err != nil {
		logrus.Debugf("df failed for %s: %v", path, err)
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 2 {
		return 0
	}
	kb, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return kb / (1024 * 1024) // KB -> GB
}

// ============================================================================	
// PG Version Resolution
// ============================================================================

func resolvePgVersion(cfg *Config) int {
	if cfg != nil && cfg.PgVersion > 0 {
		return cfg.PgVersion
	}
	pg, err := GetPgInstall(cfg)
	if err == nil && pg != nil {
		return pg.MajorVersion
	}
	logrus.Warnf("cannot detect PG version, assuming 17")
	return 17
}

// ============================================================================
// Math Helpers
// ============================================================================

func ceilInt(f float64) int  { return int(math.Ceil(f)) }
func floorInt(f float64) int { return int(math.Floor(f)) }

func roundParallel(v float64, useCeil bool) int {
	if useCeil {
		return int(math.Ceil(v))
	}
	return int(math.Floor(v))
}

// clampOpt clamps v to [lo, hi]. If hi < 0, no upper bound is applied.
func clampOpt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if hi >= 0 && v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func itoa(n int) string          { return strconv.Itoa(n) }
func mb(n int) string            { return fmt.Sprintf("%dMB", n) }
func gb(n int) string            { return fmt.Sprintf("%dGB", n) }
