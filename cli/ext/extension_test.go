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
				"1000", "timescaledb", "timescaledb", "TIME", "https://github.com/timescale/timescaledb",
				"PIGSTY", "", "2.17.2", "PIGSTY", "C", "f", "t", "t", "t", "t", "f", "f", "",
				"{17,16,15,14}", "", "2.17.2", "PIGSTY", "pg_timescaledb_$v*", "{17,16,15,14}", "",
				"2.17.2", "PIGSTY", "timescaledb-2-postgresql-$v", "", "{17,16,15,14}", "",
				"Enables scalable inserts and complex queries for time-series data", "Time-series database extension plugin",
				"degrade to oss ver on el.aarch64",
			},
			want: &Extension{
				ID:          1000,
				Name:        "timescaledb",
				Alias:       "timescaledb",
				Category:    "TIME",
				URL:         "https://github.com/timescale/timescaledb",
				License:     "PIGSTY",
				Tags:        []string{},
				Version:     "2.17.2",
				Repo:        "PIGSTY",
				Lang:        "C",
				Utility:     false,
				Lead:        true,
				HasSolib:    true,
				NeedDDL:     true,
				NeedLoad:    true,
				Trusted:     "f",
				Relocatable: "f",
				Schemas:     []string{},
				PgVer:       []string{"17", "16", "15", "14"},
				Requires:    []string{},
				RpmVer:      "2.17.2",
				RpmRepo:     "PIGSTY",
				RpmPkg:      "pg_timescaledb_$v*",
				RpmPg:       []string{"17", "16", "15", "14"},
				RpmDeps:     []string{},
				DebVer:      "2.17.2",
				DebRepo:     "PIGSTY",
				DebPkg:      "timescaledb-2-postgresql-$v",
				DebDeps:     []string{},
				DebPg:       []string{"17", "16", "15", "14"},
				BadCase:     []string{},
				EnDesc:      "Enables scalable inserts and complex queries for time-series data",
				ZhDesc:      "Time-series database extension plugin",
				Comment:     "degrade to oss ver on el.aarch64",
			},
			wantErr: false,
		},
		{
			name: "valid complex extension with multiple dependencies",
			record: []string{
				"1020", "timeseries", "pg_timeseries", "TIME", "https://github.com/ChuckHend/pg_timeseries",
				"PostgreSQL", "", "0.1.6", "PIGSTY", "SQL", "f", "t", "f", "t", "f", "f", "f", "",
				"{16,15,14,13}", "{columnar,pg_cron,pg_ivm,pg_partman}", "0.1.6", "PIGSTY", "pg_timeseries_$v",
				"{16,15,14,13}", "{hydra_$v,pg_cron_$v,pg_ivm_$v,pg_partman_$v}", "0.1.6", "PIGSTY",
				"postgresql-$v-pg-timeseries", "", "{16,15,14,13}", "",
				"Convenience API for ChuckHend time series stack", "ChuckHend time-series data API wrapper",
				"unmet deps: hydra17 not ready, pg_partman17/pg_ivm12 on el not ready",
			},
			want: &Extension{
				ID:          1020,
				Name:        "timeseries",
				Alias:       "pg_timeseries",
				Category:    "TIME",
				URL:         "https://github.com/ChuckHend/pg_timeseries",
				License:     "PostgreSQL",
				Tags:        []string{},
				Version:     "0.1.6",
				Repo:        "PIGSTY",
				Lang:        "SQL",
				Utility:     false,
				Lead:        true,
				HasSolib:    false,
				NeedDDL:     true,
				NeedLoad:    false,
				Trusted:     "f",
				Relocatable: "f",
				Schemas:     []string{},
				PgVer:       []string{"16", "15", "14", "13"},
				Requires:    []string{"columnar", "pg_cron", "pg_ivm", "pg_partman"},
				RpmVer:      "0.1.6",
				RpmRepo:     "PIGSTY",
				RpmPkg:      "pg_timeseries_$v",
				RpmPg:       []string{"16", "15", "14", "13"},
				RpmDeps:     []string{"hydra_$v", "pg_cron_$v", "pg_ivm_$v", "pg_partman_$v"},
				DebVer:      "0.1.6",
				DebRepo:     "PIGSTY",
				DebPkg:      "postgresql-$v-pg-timeseries",
				DebDeps:     []string{},
				DebPg:       []string{"16", "15", "14", "13"},
				BadCase:     []string{},
				EnDesc:      "Convenience API for ChuckHend time series stack",
				ZhDesc:      "ChuckHend time-series data API wrapper",
				Comment:     "unmet deps: hydra17 not ready, pg_partman17/pg_ivm12 on el not ready",
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
				"invalid", "test", "test", "CAT", "url", "license", "", "1.0", "repo", "lang",
				"f", "f", "f", "f", "f", "f", "f", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
				"desc", "desc", "comment",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty fields are handled correctly",
			record: []string{
				"1000", "", "", "", "", "", "", "", "", "", "f", "f", "f", "f", "f", "", "", "", "", "",
				"", "", "", "", "", "", "", "", "", "", "", "", "", "",
			},
			want: &Extension{
				ID:          1000,
				Name:        "",
				Alias:       "",
				Category:    "",
				URL:         "",
				License:     "",
				Tags:        []string{},
				Version:     "",
				Repo:        "",
				Lang:        "",
				Utility:     false,
				Lead:        false,
				HasSolib:    false,
				NeedDDL:     false,
				NeedLoad:    false,
				Trusted:     "",
				Relocatable: "",
				Schemas:     []string{},
				PgVer:       []string{},
				Requires:    []string{},
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
				BadCase:     []string{},
				EnDesc:      "",
				ZhDesc:      "",
				Comment:     "",
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
