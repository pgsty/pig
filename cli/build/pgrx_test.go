package build

import (
	"fmt"
	"reflect"
	"testing"

	"pig/cli/ext"
)

func TestPgrxAutoDetectVersionStringsAddsBetaOnlyWhenRequested(t *testing.T) {
	oldActive := ext.PostgresActiveMajorVersions
	ext.PostgresActiveMajorVersions = []int{18, 17, 16, 15, 14}
	defer func() {
		ext.PostgresActiveMajorVersions = oldActive
	}()

	wantDefault := []string{"18", "17", "16", "15", "14"}
	if got := pgrxAutoDetectVersionStrings(false); !reflect.DeepEqual(got, wantDefault) {
		t.Fatalf("pgrxAutoDetectVersionStrings(default) = %v, want %v", got, wantDefault)
	}

	wantBeta := []string{fmt.Sprintf("%d", ext.PostgresBetaMajorVersion), "18", "17", "16", "15", "14"}
	if got := pgrxAutoDetectVersionStrings(true); !reflect.DeepEqual(got, wantBeta) {
		t.Fatalf("pgrxAutoDetectVersionStrings(beta) = %v, want %v", got, wantBeta)
	}
}

func TestSplitPgrxVersionStringsKeepsExplicitPgList(t *testing.T) {
	got := splitPgrxVersionStrings("18, 17")
	want := []string{"18", "17"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitPgrxVersionStrings(explicit) = %v, want %v", got, want)
	}
}
