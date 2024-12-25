package ext

import (
	"fmt"
	"pig/internal/config"
	"strconv"
)

var DistroBadCase = map[string]map[string][]int{
	"el8.amd64": {"pljava": {}, "timescaledb_toolkit": {}},
	"el9.amd64": {},
	"u24.amd64": {"pgml": {}, "citus": {}, "topn": {}, "timescaledb_toolkit": {}, "pg_partman": {13}, "timeseries": {13}},
	"u22.amd64": {},
	"d12.amd64": {"babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}},
	"el8.arm64": {"pg_dbms_job": {}, "pljava": {}, "timescaledb_toolkit": {}, "jdbc_fdw": {}, "pllua": {15, 14, 13}, "topn": {13}},
	"el9.arm64": {"pg_dbms_job": {}, "timescaledb_toolkit": {}, "jdbc_fdw": {}, "pllua": {15, 14, 13}, "topn": {13}},
	"u24.arm64": {"pgml": {}, "citus": {}, "topn": {}, "timescaledb_toolkit": {}, "pg_partman": {13}, "timeseries": {13}},
	"u22.arm64": {"topn": {}},
	"d12.arm64": {"topn": {}, "babelfishpg_common": {}, "babelfishpg_tsql": {}, "babelfishpg_tds": {}, "babelfishpg_money": {}},
}

var RpmRenameMap = map[string]map[int]string{
	"pgaudit": {15: "pgaudit17_15*", 14: "pgaudit17_14*", 13: "pgaudit17_13*"},
}

// Available check if the extension is available for the given pg version
func (e *Extension) Available(pgVer int) bool {
	verStr := strconv.Itoa(pgVer)

	// test1: check rpm/deb version compatibility
	switch config.OSType {
	case config.DistroEL:
		if e.RpmPg != nil {
			found := false
			for _, ver := range e.RpmPg {
				if ver == verStr {
					found = true
					continue
				}
			}
			if !found {
				return false
			}
		}
	case config.DistroDEB:
		if e.DebPg != nil {
			found := false
			for _, ver := range e.DebPg {
				if ver == verStr {
					found = true
					continue
				}
			}
			if !found {
				return false
			}
		}
	case config.DistroMAC:
		return true
	}

	// test2 will check bad base according to DistroCode and OSArch
	distroCodeArch := fmt.Sprintf("%s.%s", config.OSCode, config.OSArch)
	badCases := DistroBadCase[distroCodeArch]
	if badCases == nil {
		return true
	}
	v, ok := badCases[e.Name]
	if !ok {
		return true
	} else {
		if len(v) == 0 { // match all version
			return false
		}
		for _, ver := range v {
			if ver == pgVer {
				return false
			}
		}
		return true
	}
}
