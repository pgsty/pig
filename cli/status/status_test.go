package status

import (
	"pig/internal/config"
	"testing"
)

func TestGetStatusResult(t *testing.T) {
	result := GetStatusResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success=true, got false: %s", result.Message)
	}

	data, ok := result.Data.(*StatusData)
	if !ok {
		t.Fatalf("expected data to be *StatusData, got %T", result.Data)
	}

	if data.Pig.Version != config.PigVersion {
		t.Errorf("expected Pig.Version=%s, got %s", config.PigVersion, data.Pig.Version)
	}
	if data.Host.OS != config.GOOS {
		t.Errorf("expected Host.OS=%s, got %s", config.GOOS, data.Host.OS)
	}
}

func TestGetVersionResult(t *testing.T) {
	result := GetVersionResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Fatalf("expected success=true, got false: %s", result.Message)
	}

	data, ok := result.Data.(*VersionData)
	if !ok {
		t.Fatalf("expected data to be *VersionData, got %T", result.Data)
	}

	if data.Version != config.PigVersion {
		t.Errorf("expected Version=%s, got %s", config.PigVersion, data.Version)
	}
	if data.OS != config.GOOS {
		t.Errorf("expected OS=%s, got %s", config.GOOS, data.OS)
	}
}
