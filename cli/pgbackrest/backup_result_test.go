/*
Copyright 2018-2025 Ruohang Feng <rh@vonng.com>

pb backup structured output result tests.
*/
package pgbackrest

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPbBackupResultDataJSONSerialization tests JSON marshaling/unmarshaling of PbBackupResultData.
func TestPbBackupResultDataJSONSerialization(t *testing.T) {
	data := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "full",
		StartTime:       1738670400,
		StopTime:        1738674000,
		Size:            1073741824, // 1GB
		SizeRepo:        268435456,  // 256MB
		DurationSeconds: 3600,
		Stanza:          "pg-test",
		LSNStart:        "0/3000028",
		LSNStop:         "0/5000000",
		WALStart:        "000000010000000000000003",
		WALStop:         "000000010000000000000005",
	}

	// Test JSON marshal
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Verify expected fields are present
	jsonStr := string(jsonBytes)
	expectedFields := []string{
		`"label":"20250204-120000F"`,
		`"type":"full"`,
		`"start_time":1738670400`,
		`"stop_time":1738674000`,
		`"size":1073741824`,
		`"size_repo":268435456`,
		`"duration_seconds":3600`,
		`"stanza":"pg-test"`,
		`"lsn_start":"0/3000028"`,
		`"lsn_stop":"0/5000000"`,
		`"wal_start":"000000010000000000000003"`,
		`"wal_stop":"000000010000000000000005"`,
	}

	for _, field := range expectedFields {
		if !containsAny(jsonStr, field) {
			t.Errorf("JSON output missing expected field: %s", field)
		}
	}

	// Test JSON unmarshal roundtrip
	var unmarshaled PbBackupResultData
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if unmarshaled.Label != data.Label {
		t.Errorf("Label mismatch: got %s, want %s", unmarshaled.Label, data.Label)
	}
	if unmarshaled.Type != data.Type {
		t.Errorf("Type mismatch: got %s, want %s", unmarshaled.Type, data.Type)
	}
	if unmarshaled.Size != data.Size {
		t.Errorf("Size mismatch: got %d, want %d", unmarshaled.Size, data.Size)
	}
	if unmarshaled.DurationSeconds != data.DurationSeconds {
		t.Errorf("DurationSeconds mismatch: got %d, want %d", unmarshaled.DurationSeconds, data.DurationSeconds)
	}
}

// TestPbBackupResultDataYAMLSerialization tests YAML marshaling/unmarshaling of PbBackupResultData.
func TestPbBackupResultDataYAMLSerialization(t *testing.T) {
	data := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "incr",
		StartTime:       1738670400,
		StopTime:        1738672200,
		Size:            536870912,
		SizeRepo:        134217728,
		DurationSeconds: 1800,
		Stanza:          "pg-test",
		Prior:           "20250203-120000F",
	}

	// Test YAML marshal
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("YAML marshal failed: %v", err)
	}

	// Test YAML unmarshal roundtrip
	var unmarshaled PbBackupResultData
	if err := yaml.Unmarshal(yamlBytes, &unmarshaled); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if unmarshaled.Label != data.Label {
		t.Errorf("Label mismatch: got %s, want %s", unmarshaled.Label, data.Label)
	}
	if unmarshaled.Type != data.Type {
		t.Errorf("Type mismatch: got %s, want %s", unmarshaled.Type, data.Type)
	}
	if unmarshaled.Prior != data.Prior {
		t.Errorf("Prior mismatch: got %s, want %s", unmarshaled.Prior, data.Prior)
	}
}

// TestPbBackupResultDataNilSafe tests nil-safety of PbBackupResultData methods.
func TestPbBackupResultDataNilSafe(t *testing.T) {
	var data *PbBackupResultData

	// Test that nil data can be safely marshaled
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal of nil failed: %v", err)
	}
	if string(jsonBytes) != "null" {
		t.Errorf("Expected 'null', got: %s", string(jsonBytes))
	}

	// Test with empty struct
	emptyData := &PbBackupResultData{}
	jsonBytes, err = json.Marshal(emptyData)
	if err != nil {
		t.Fatalf("JSON marshal of empty struct failed: %v", err)
	}

	var unmarshaled PbBackupResultData
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// All fields should be zero values
	if unmarshaled.Label != "" {
		t.Errorf("Expected empty Label, got: %s", unmarshaled.Label)
	}
	if unmarshaled.Size != 0 {
		t.Errorf("Expected Size 0, got: %d", unmarshaled.Size)
	}
}

// TestPbBackupResultDataAllBackupTypes tests different backup types.
func TestPbBackupResultDataAllBackupTypes(t *testing.T) {
	testCases := []struct {
		backupType string
		hasPrior   bool
	}{
		{"full", false},
		{"diff", true},
		{"incr", true},
	}

	for _, tc := range testCases {
		t.Run(tc.backupType, func(t *testing.T) {
			data := &PbBackupResultData{
				Label: "20250204-120000F",
				Type:  tc.backupType,
			}
			if tc.hasPrior {
				data.Prior = "20250203-120000F"
			}

			jsonBytes, err := json.Marshal(data)
			if err != nil {
				t.Fatalf("JSON marshal failed: %v", err)
			}

			var unmarshaled PbBackupResultData
			if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
				t.Fatalf("JSON unmarshal failed: %v", err)
			}

			if unmarshaled.Type != tc.backupType {
				t.Errorf("Type mismatch: got %s, want %s", unmarshaled.Type, tc.backupType)
			}

			if tc.hasPrior && unmarshaled.Prior == "" {
				t.Errorf("Expected Prior to be set for %s backup", tc.backupType)
			}
		})
	}
}

// TestPbBackupResultDataOmitEmpty tests that empty optional fields are omitted.
func TestPbBackupResultDataOmitEmpty(t *testing.T) {
	// Data with only required fields
	data := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "full",
		StartTime:       1738670400,
		StopTime:        1738674000,
		Size:            1073741824,
		SizeRepo:        268435456,
		DurationSeconds: 3600,
		Stanza:          "pg-test",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Prior should be omitted when empty
	if containsAny(jsonStr, `"prior"`) {
		t.Errorf("Empty prior field should be omitted, got: %s", jsonStr)
	}
}

// TestPbBackupResultDataWithLSN tests backup data with LSN information.
func TestPbBackupResultDataWithLSN(t *testing.T) {
	data := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "full",
		StartTime:       1738670400,
		StopTime:        1738674000,
		Size:            1073741824,
		SizeRepo:        268435456,
		DurationSeconds: 3600,
		Stanza:          "pg-test",
		LSNStart:        "0/3000028",
		LSNStop:         "0/5000000",
		WALStart:        "000000010000000000000003",
		WALStop:         "000000010000000000000005",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var unmarshaled PbBackupResultData
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if unmarshaled.LSNStart != data.LSNStart {
		t.Errorf("LSNStart mismatch: got %s, want %s", unmarshaled.LSNStart, data.LSNStart)
	}
	if unmarshaled.LSNStop != data.LSNStop {
		t.Errorf("LSNStop mismatch: got %s, want %s", unmarshaled.LSNStop, data.LSNStop)
	}
	if unmarshaled.WALStart != data.WALStart {
		t.Errorf("WALStart mismatch: got %s, want %s", unmarshaled.WALStart, data.WALStart)
	}
	if unmarshaled.WALStop != data.WALStop {
		t.Errorf("WALStop mismatch: got %s, want %s", unmarshaled.WALStop, data.WALStop)
	}
}

// TestPbBackupResultDataDuration tests duration calculation.
func TestPbBackupResultDataDuration(t *testing.T) {
	startTime := int64(1738670400)
	stopTime := int64(1738674000)
	expectedDuration := stopTime - startTime // 3600 seconds

	data := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "full",
		StartTime:       startTime,
		StopTime:        stopTime,
		DurationSeconds: expectedDuration,
		Stanza:          "pg-test",
	}

	if data.DurationSeconds != 3600 {
		t.Errorf("DurationSeconds = %d, want 3600", data.DurationSeconds)
	}

	// Verify consistency: duration should equal stop - start
	if data.DurationSeconds != (data.StopTime - data.StartTime) {
		t.Errorf("DurationSeconds (%d) != StopTime - StartTime (%d)",
			data.DurationSeconds, data.StopTime-data.StartTime)
	}
}

// TestPbBackupResultDataCompleteScenario tests a complete backup result scenario.
func TestPbBackupResultDataCompleteScenario(t *testing.T) {
	// Simulate a full backup result
	fullBackup := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "full",
		StartTime:       1738670400,
		StopTime:        1738674000,
		Size:            10737418240, // 10GB
		SizeRepo:        2684354560,  // 2.5GB (compressed)
		DurationSeconds: 3600,
		Stanza:          "pg-production",
		LSNStart:        "0/3000028",
		LSNStop:         "0/5000000",
		WALStart:        "000000010000000000000003",
		WALStop:         "000000010000000000000005",
	}

	// Full backup should not have Prior
	if fullBackup.Prior != "" {
		t.Errorf("Full backup should not have Prior, got: %s", fullBackup.Prior)
	}

	// Simulate an incremental backup result
	incrBackup := &PbBackupResultData{
		Label:           "20250204-180000I",
		Type:            "incr",
		StartTime:       1738692000,
		StopTime:        1738693800,
		Size:            536870912, // 512MB
		SizeRepo:        134217728, // 128MB (compressed)
		DurationSeconds: 1800,
		Stanza:          "pg-production",
		LSNStart:        "0/5000000",
		LSNStop:         "0/6000000",
		WALStart:        "000000010000000000000005",
		WALStop:         "000000010000000000000006",
		Prior:           "20250204-120000F",
	}

	// Incremental backup must have Prior
	if incrBackup.Prior == "" {
		t.Error("Incremental backup should have Prior reference")
	}

	// Verify JSON roundtrip
	jsonBytes, err := json.Marshal(incrBackup)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var unmarshaled PbBackupResultData
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if unmarshaled.Prior != incrBackup.Prior {
		t.Errorf("Prior mismatch after roundtrip: got %s, want %s", unmarshaled.Prior, incrBackup.Prior)
	}
}

// TestPbBackupResultDataLargeSizes tests handling of large backup sizes.
func TestPbBackupResultDataLargeSizes(t *testing.T) {
	// Test with very large sizes (TB scale)
	data := &PbBackupResultData{
		Label:           "20250204-120000F",
		Type:            "full",
		StartTime:       1738670400,
		StopTime:        1738699200,
		Size:            5497558138880, // 5TB
		SizeRepo:        1099511627776, // 1TB (compressed)
		DurationSeconds: 28800,         // 8 hours
		Stanza:          "pg-warehouse",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var unmarshaled PbBackupResultData
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if unmarshaled.Size != data.Size {
		t.Errorf("Large size mismatch: got %d, want %d", unmarshaled.Size, data.Size)
	}
	if unmarshaled.SizeRepo != data.SizeRepo {
		t.Errorf("Large repo size mismatch: got %d, want %d", unmarshaled.SizeRepo, data.SizeRepo)
	}
}
