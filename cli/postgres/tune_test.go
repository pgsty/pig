package postgres

import (
	"testing"
)

// paramMap converts a TuneParam slice to name→value map for easy assertions.
func paramMap(params []TuneParam) map[string]string {
	m := make(map[string]string)
	for _, p := range params {
		m[p.Name] = p.Value
	}
	return m
}

func assertEqual(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", name, got, want)
	}
}

// ============================================================================
// OLTP Profile
// ============================================================================

func TestCalculateTuneParams_OLTP_8C32G500G(t *testing.T) {
	spec := TuneSpec{CPU: 8, MemMB: 32768, DiskGB: 500}
	params := CalculateTuneParams(spec, tuneProfiles["oltp"], 100, 0.25, 17)
	m := paramMap(params)

	assertEqual(t, "shared_buffers", m["shared_buffers"], "8192MB")
	assertEqual(t, "maintenance_work_mem", m["maintenance_work_mem"], "2048MB")
	assertEqual(t, "work_mem", m["work_mem"], "81MB")       // floor(8192/100)=81
	assertEqual(t, "effective_cache_size", m["effective_cache_size"], "24576MB")
	assertEqual(t, "huge_pages", m["huge_pages"], "try")
	assertEqual(t, "hash_mem_multiplier", m["hash_mem_multiplier"], "8.0")
	assertEqual(t, "max_connections", m["max_connections"], "100")

	assertEqual(t, "max_worker_processes", m["max_worker_processes"], "16") // max(8+8,16)=16
	assertEqual(t, "max_parallel_workers", m["max_parallel_workers"], "4") // ceil(8*0.5)=4
	assertEqual(t, "max_parallel_workers_per_gather", m["max_parallel_workers_per_gather"], "2") // clamp(floor(8*0.2)=1,2,8)=2
	assertEqual(t, "max_parallel_maintenance_workers", m["max_parallel_maintenance_workers"], "3") // ceil(8*0.33)=3

	// size_twentieth = clamp(ceil(500/20),1,100) = 25
	assertEqual(t, "min_wal_size", m["min_wal_size"], "25GB")
	assertEqual(t, "max_wal_size", m["max_wal_size"], "100GB")      // min(25*4,2000)
	assertEqual(t, "max_slot_wal_keep_size", m["max_slot_wal_keep_size"], "150GB") // min(25*6,3000)
	assertEqual(t, "temp_file_limit", m["temp_file_limit"], "25GB") // min(25*1,200)

	assertEqual(t, "random_page_cost", m["random_page_cost"], "1.1")
	assertEqual(t, "effective_io_concurrency", m["effective_io_concurrency"], "200")
	assertEqual(t, "maintenance_io_concurrency", m["maintenance_io_concurrency"], "100")
}

// ============================================================================
// OLAP Profile
// ============================================================================

func TestCalculateTuneParams_OLAP_64C256G2T(t *testing.T) {
	spec := TuneSpec{CPU: 64, MemMB: 262144, DiskGB: 2000}
	params := CalculateTuneParams(spec, tuneProfiles["olap"], 100, 0.25, 17)
	m := paramMap(params)

	assertEqual(t, "shared_buffers", m["shared_buffers"], "65536MB")
	assertEqual(t, "maintenance_work_mem", m["maintenance_work_mem"], "32768MB") // SB*0.5
	assertEqual(t, "work_mem", m["work_mem"], "655MB")                          // floor(65536/100)=655
	assertEqual(t, "max_worker_processes", m["max_worker_processes"], "76")      // max(64+12,20)=76
	assertEqual(t, "max_parallel_workers", m["max_parallel_workers"], "52")      // ceil(64*0.8)=52
	assertEqual(t, "max_parallel_workers_per_gather", m["max_parallel_workers_per_gather"], "32") // floor(64*0.5)=32, no cap
	assertEqual(t, "max_parallel_maintenance_workers", m["max_parallel_maintenance_workers"], "22") // ceil(64*0.33)=22

	// size_twentieth = clamp(ceil(2000/20),1,100) = 100
	assertEqual(t, "temp_file_limit", m["temp_file_limit"], "400GB") // min(100*4,2000)
}

// ============================================================================
// TINY Profile
// ============================================================================

func TestCalculateTuneParams_Tiny_2C2G50G(t *testing.T) {
	spec := TuneSpec{CPU: 2, MemMB: 2048, DiskGB: 50}
	params := CalculateTuneParams(spec, tuneProfiles["tiny"], 100, 0.25, 17)
	m := paramMap(params)

	assertEqual(t, "shared_buffers", m["shared_buffers"], "512MB")
	assertEqual(t, "max_parallel_workers", m["max_parallel_workers"], "1")                        // floor(2*0.5)=1
	assertEqual(t, "max_parallel_workers_per_gather", m["max_parallel_workers_per_gather"], "0")   // fixed 0
	assertEqual(t, "max_parallel_maintenance_workers", m["max_parallel_maintenance_workers"], "1") // floor(2*0.33)=0, max(0,1)=1
	assertEqual(t, "work_mem", m["work_mem"], "16MB")                                             // floor(512/100)=5, clamp(5,16,256)=16
	assertEqual(t, "max_worker_processes", m["max_worker_processes"], "12")                        // max(2+4,12)=12
}

// ============================================================================
// CRIT Profile
// ============================================================================

func TestCalculateTuneParams_Crit_16C64G1T(t *testing.T) {
	spec := TuneSpec{CPU: 16, MemMB: 65536, DiskGB: 1000}
	params := CalculateTuneParams(spec, tuneProfiles["crit"], 100, 0.25, 17)
	m := paramMap(params)

	assertEqual(t, "max_parallel_workers_per_gather", m["max_parallel_workers_per_gather"], "0") // fixed 0
	assertEqual(t, "max_worker_processes", m["max_worker_processes"], "24")                      // max(16+8,16)=24
	assertEqual(t, "max_parallel_workers", m["max_parallel_workers"], "8")                       // ceil(16*0.5)=8
}

// ============================================================================
// PG Version Tests
// ============================================================================

func TestCalculateTuneParams_PG18IOWorkers(t *testing.T) {
	spec := TuneSpec{CPU: 16, MemMB: 65536, DiskGB: 1000}
	params := CalculateTuneParams(spec, tuneProfiles["oltp"], 100, 0.25, 18)
	m := paramMap(params)

	assertEqual(t, "io_workers", m["io_workers"], "4") // clamp(ceil(16*0.25)=4,4,16)=4
	assertEqual(t, "hash_mem_multiplier", m["hash_mem_multiplier"], "8.0")
}

func TestCalculateTuneParams_PG12NoExtras(t *testing.T) {
	spec := TuneSpec{CPU: 4, MemMB: 8192, DiskGB: 200}
	params := CalculateTuneParams(spec, tuneProfiles["oltp"], 100, 0.25, 12)
	m := paramMap(params)

	if _, ok := m["hash_mem_multiplier"]; ok {
		t.Error("hash_mem_multiplier should not be present for PG 12")
	}
	if _, ok := m["io_workers"]; ok {
		t.Error("io_workers should not be present for PG 12")
	}
	if _, ok := m["maintenance_io_concurrency"]; ok {
		t.Error("maintenance_io_concurrency should not be present for PG 12")
	}
}

// ============================================================================
// Boundary & Override Tests
// ============================================================================

func TestCalculateTuneParams_MinimalHardware(t *testing.T) {
	spec := TuneSpec{CPU: 1, MemMB: 512, DiskGB: 10}
	params := CalculateTuneParams(spec, tuneProfiles["tiny"], 100, 0.25, 13)
	for _, p := range params {
		if p.Value == "" {
			t.Errorf("empty value for %s", p.Name)
		}
	}
}

func TestCalculateTuneParams_MaxConnOverride(t *testing.T) {
	spec := TuneSpec{CPU: 8, MemMB: 32768, DiskGB: 500}
	params := CalculateTuneParams(spec, tuneProfiles["oltp"], 500, 0.25, 17)
	m := paramMap(params)

	assertEqual(t, "max_connections", m["max_connections"], "500")
	assertEqual(t, "work_mem", m["work_mem"], "64MB") // floor(8192/500)=16, clamp(16,64,1024)=64
}

func TestCalculateTuneParams_ShmemRatioOverride(t *testing.T) {
	spec := TuneSpec{CPU: 8, MemMB: 32768, DiskGB: 500}
	params := CalculateTuneParams(spec, tuneProfiles["oltp"], 100, 0.3, 17)
	m := paramMap(params)

	assertEqual(t, "shared_buffers", m["shared_buffers"], "9831MB") // ceil(32768*0.3)=9831
}

// ============================================================================
// Math Helper Tests
// ============================================================================

func TestClampOpt(t *testing.T) {
	if clampOpt(5, 2, 10) != 5 {
		t.Error("normal case")
	}
	if clampOpt(1, 2, 10) != 2 {
		t.Error("below min")
	}
	if clampOpt(15, 2, 10) != 10 {
		t.Error("above max")
	}
	if clampOpt(100, 2, -1) != 100 {
		t.Error("no upper bound")
	}
	if clampOpt(1, 2, -1) != 2 {
		t.Error("below min, no upper bound")
	}
}

func TestRoundParallel(t *testing.T) {
	if roundParallel(3.2, true) != 4 {
		t.Error("ceil 3.2 should be 4")
	}
	if roundParallel(3.2, false) != 3 {
		t.Error("floor 3.2 should be 3")
	}
}
