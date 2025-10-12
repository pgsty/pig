package ext

import (
	"reflect"
	"testing"
)

func TestParseExtension(t *testing.T) {
	tests := []struct {
		name    string
		record  []string
		want    *Extension
		wantErr bool
	}{
		{
			name: "valid timescaledb extension",
			record: []string{
				"1000", "timescaledb", "timescaledb", "timescaledb", "TIME", "available",
				"https://github.com/timescale/timescaledb", "Timescale", "", "2.22.0", "PIGSTY", "C",
				"f", "t", "f", "t", "t", "t", "f", "f", "{timescaledb_information,timescaledb_experimental}",
				"{17,16,15}", "", "", "",
				"2.22.0", "PIGSTY", "timescaledb-tsl_$v*", "{17,16,15,14}", "",
				"2.22.0", "PIGSTY", "postgresql-$v-timescaledb-tsl", "", "{17,16,15,14}",
				"timescaledb-2.22.0.tar.gz", "{}",
				"Enables scalable inserts and complex queries for time-series data",
				"时序数据库扩展插件", "", "2025-07-24",
			},
			want: &Extension{
				ID:          1000,
				Name:        "timescaledb",
				Pkg:         "timescaledb",
				LeadExt:     "timescaledb",
				Category:    "TIME",
				State:       "available",
				URL:         "https://github.com/timescale/timescaledb",
				License:     "Timescale",
				Tags:        []string{},
				Version:     "2.22.0",
				Repo:        "PIGSTY",
				Lang:        "C",
				Contrib:     false,
				Lead:        true,
				HasBin:      false,
				HasLib:      true,
				NeedDDL:     true,
				NeedLoad:    true,
				Trusted:     "f",
				Relocatable: "f",
				Schemas:     []string{"timescaledb_information", "timescaledb_experimental"},
				PgVer:       []string{"17", "16", "15"},
				Requires:    []string{},
				RequireBy:   []string{},
				SeeAlso:     []string{},
				RpmVer:      "2.22.0",
				RpmRepo:     "PIGSTY",
				RpmPkg:      "timescaledb-tsl_$v*",
				RpmPg:       []string{"17", "16", "15", "14"},
				RpmDeps:     []string{},
				DebVer:      "2.22.0",
				DebRepo:     "PIGSTY",
				DebPkg:      "postgresql-$v-timescaledb-tsl",
				DebDeps:     []string{},
				DebPg:       []string{"17", "16", "15", "14"},
				Source:      "timescaledb-2.22.0.tar.gz",
				Extra:       map[string]interface{}{},
				EnDesc:      "Enables scalable inserts and complex queries for time-series data",
				ZhDesc:      "时序数据库扩展插件",
				Comment:     "",
				Mtime:       "2025-07-24",
			},
			wantErr: false,
		},
		{
			name: "valid complex extension with multiple dependencies",
			record: []string{
				"1020", "timeseries", "pg_timeseries", "timeseries", "TIME", "available",
				"https://github.com/ChuckHend/pg_timeseries", "PostgreSQL", "", "0.1.6", "PIGSTY", "SQL",
				"f", "t", "f", "f", "t", "f", "f", "f", "",
				"{17,16,15,14,13}", "{columnar,pg_cron,pg_ivm,pg_partman}", "", "{timescaledb,timescaledb_toolkit}",
				"0.1.6", "PIGSTY", "pg_timeseries_$v", "{17,16,15,14,13}", "{hydra_$v,pg_cron_$v,pg_ivm_$v,pg_partman_$v}",
				"0.1.6", "PIGSTY", "postgresql-$v-pg-timeseries", "", "{17,16,15,14,13}",
				"pg_timeseries-0.1.6.tar.gz", "{}",
				"Convenience API for time series stack", "时序数据API封装",
				"unmet deps: hydra17 not ready", "2025-02-20",
			},
			want: &Extension{
				ID:          1020,
				Name:        "timeseries",
				Pkg:         "pg_timeseries",
				LeadExt:     "timeseries",
				Category:    "TIME",
				State:       "available",
				URL:         "https://github.com/ChuckHend/pg_timeseries",
				License:     "PostgreSQL",
				Tags:        []string{},
				Version:     "0.1.6",
				Repo:        "PIGSTY",
				Lang:        "SQL",
				Contrib:     false,
				Lead:        true,
				HasBin:      false,
				HasLib:      false,
				NeedDDL:     true,
				NeedLoad:    false,
				Trusted:     "f",
				Relocatable: "f",
				Schemas:     []string{},
				PgVer:       []string{"17", "16", "15", "14", "13"},
				Requires:    []string{"columnar", "pg_cron", "pg_ivm", "pg_partman"},
				RequireBy:   []string{},
				SeeAlso:     []string{"timescaledb", "timescaledb_toolkit"},
				RpmVer:      "0.1.6",
				RpmRepo:     "PIGSTY",
				RpmPkg:      "pg_timeseries_$v",
				RpmPg:       []string{"17", "16", "15", "14", "13"},
				RpmDeps:     []string{"hydra_$v", "pg_cron_$v", "pg_ivm_$v", "pg_partman_$v"},
				DebVer:      "0.1.6",
				DebRepo:     "PIGSTY",
				DebPkg:      "postgresql-$v-pg-timeseries",
				DebDeps:     []string{},
				DebPg:       []string{"17", "16", "15", "14", "13"},
				Source:      "pg_timeseries-0.1.6.tar.gz",
				Extra:       map[string]interface{}{},
				EnDesc:      "Convenience API for time series stack",
				ZhDesc:      "时序数据API封装",
				Comment:     "unmet deps: hydra17 not ready",
				Mtime:       "2025-02-20",
			},
			wantErr: false,
		},
		{
			name:    "invalid record length",
			record:  []string{"1", "test"},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid ID",
			record: []string{
				"invalid", "test", "test", "test", "CAT", "state", "url", "license", "", "1.0", "repo", "lang",
				"f", "f", "f", "f", "f", "f", "f", "f", "", "", "", "", "",
				"", "", "", "", "", "", "", "", "", "", "", "{}",
				"desc", "desc", "comment", "2025-01-01",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty fields are handled correctly",
			record: []string{
				"1000", "", "", "", "", "", "", "", "", "", "", "",
				"f", "f", "f", "f", "f", "f", "", "", "", "", "", "", "",
				"", "", "", "", "", "", "", "", "", "", "", "{}",
				"", "", "", "",
			},
			want: &Extension{
				ID:          1000,
				Name:        "",
				Pkg:         "",
				LeadExt:     "",
				Category:    "",
				State:       "",
				URL:         "",
				License:     "",
				Tags:        []string{},
				Version:     "",
				Repo:        "",
				Lang:        "",
				Contrib:     false,
				Lead:        false,
				HasBin:      false,
				HasLib:      false,
				NeedDDL:     false,
				NeedLoad:    false,
				Trusted:     "",
				Relocatable: "",
				Schemas:     []string{},
				PgVer:       []string{},
				Requires:    []string{},
				RequireBy:   []string{},
				SeeAlso:     []string{},
				RpmVer:      "",
				RpmRepo:     "",
				RpmPkg:      "",
				RpmPg:       []string{},
				RpmDeps:     []string{},
				DebVer:      "",
				DebRepo:     "",
				DebPkg:      "",
				DebDeps:     []string{},
				DebPg:       []string{},
				Source:      "",
				Extra:       map[string]interface{}{},
				EnDesc:      "",
				ZhDesc:      "",
				Comment:     "",
				Mtime:       "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExtension(tt.record)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseExtension() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseExtension() = %v, want %v", got, tt.want)
			}
		})
	}
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
			want: []string{},
		},
		{
			name: "empty array",
			s:    "{}",
			want: []string{},
		},
		{
			name: "single value",
			s:    "{value}",
			want: []string{"value"},
		},
		{
			name: "multiple values",
			s:    "{value1,value2,value3}",
			want: []string{"value1", "value2", "value3"},
		},
		{
			name: "values with spaces",
			s:    "{ value1 , value2 , value3 }",
			want: []string{"value1", "value2", "value3"},
		},
		{
			name: "values with empty elements",
			s:    "{value1,,value3}",
			want: []string{"value1", "value3"},
		},
		{
			name: "no braces",
			s:    "value1,value2,value3",
			want: []string{"value1", "value2", "value3"},
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
