package pgext

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseExtension2(t *testing.T) {
	InitExtensionData("")

	//TabulateExtension(nil)
	//TabulateExtension(FilterByDistro("el9"))
	TabulateExtension(FilterByCategory("TIME"))

	fmt.Println(NeedBy)
}

func TestExtensionInfo(t *testing.T) {
	InitExtensionData("")
	ExtNameMap["postgis"].PrintInfo()
	ExtNameMap["timescaledb"].PrintInfo()
	ExtNameMap["postgis_tiger_geocoder"].PrintInfo()
	ExtNameMap["vchord"].PrintInfo()

	//ExtNameMap["http"].PrintInfo()
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want []string
	}{
		{
			name: "empty string",
			s:    "",
			want: nil,
		},
		{
			name: "single value",
			s:    "value",
			want: []string{"value"},
		},
		{
			name: "multiple values",
			s:    "value1,value2,value3",
			want: []string{"value1", "value2", "value3"},
		},
		{
			name: "values with spaces",
			s:    " value1 , value2 , value3 ",
			want: []string{"value1", "value2", "value3"},
		},
		{
			name: "empty values",
			s:    "value1,,value3",
			want: []string{"value1", "value3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitAndTrim(tt.s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitAndTrim() = %v, want %v", got, tt.want)
			}
		})
	}
}
