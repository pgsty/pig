package sty

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const maxDashboardDiffPaths = 256

type normalizedDashboard struct {
	Value     map[string]interface{}
	Canonical []byte
}

func normalizeDashboardJSON(value map[string]interface{}) (normalizedDashboard, error) {
	if wrapped, ok := value["dashboard"].(map[string]interface{}); ok {
		value = wrapped
	}
	clone, err := cloneJSONObject(value)
	if err != nil {
		return normalizedDashboard{}, err
	}
	delete(clone, "id")
	delete(clone, "version")
	delete(clone, "iteration")
	removeDefaultJSONField(clone, "preload", false)
	removeDefaultJSONField(clone, "liveNow", false)
	removeDefaultJSONField(clone, "weekStart", "")
	removeDefaultJSONField(clone, "fiscalYearStartMonth", json.Number("0"))
	if templating, ok := clone["templating"].(map[string]interface{}); ok {
		if list, ok := templating["list"].([]interface{}); ok {
			for _, item := range list {
				if variable, ok := item.(map[string]interface{}); ok {
					delete(variable, "current")
				}
			}
		}
	}
	canonical, err := json.Marshal(clone)
	if err != nil {
		return normalizedDashboard{}, fmt.Errorf("encode normalized dashboard: %w", err)
	}
	return normalizedDashboard{Value: clone, Canonical: canonical}, nil
}

func desiredDatasourceValue(item DashboardManifestDatasource) (map[string]interface{}, error) {
	value, err := cloneJSONObject(item.Spec)
	if err != nil {
		return nil, err
	}
	value["uid"] = item.UID
	value["name"] = item.Name
	value["type"] = item.Type
	return value, nil
}

func normalizeDatasourceAgainst(expected, live map[string]interface{}) ([]byte, []byte, []string, error) {
	expectedClone, err := cloneJSONObject(expected)
	if err != nil {
		return nil, nil, nil, err
	}
	liveProjection := projectJSONShape(expectedClone, live)
	expectedCanonical, err := json.Marshal(expectedClone)
	if err != nil {
		return nil, nil, nil, err
	}
	liveCanonical, err := json.Marshal(liveProjection)
	if err != nil {
		return nil, nil, nil, err
	}
	return expectedCanonical, liveCanonical, diffJSONPointers(expectedClone, liveProjection), nil
}

func projectJSONShape(shape, actual map[string]interface{}) map[string]interface{} {
	projected := make(map[string]interface{}, len(shape))
	for key, desired := range shape {
		live, exists := actual[key]
		if !exists {
			continue
		}
		desiredMap, desiredIsMap := desired.(map[string]interface{})
		liveMap, liveIsMap := live.(map[string]interface{})
		if desiredIsMap && liveIsMap {
			projected[key] = projectJSONShape(desiredMap, liveMap)
		} else {
			projected[key] = live
		}
	}
	return projected
}

func cloneJSONObject(value map[string]interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	clone := make(map[string]interface{})
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func removeDefaultJSONField(object map[string]interface{}, key string, expected interface{}) {
	value, exists := object[key]
	if !exists {
		return
	}
	if jsonValuesEqual(value, expected) {
		delete(object, key)
	}
}

func jsonValuesEqual(left, right interface{}) bool {
	leftNumber, leftIsNumber := left.(json.Number)
	rightNumber, rightIsNumber := right.(json.Number)
	if leftIsNumber && rightIsNumber {
		return leftNumber.String() == rightNumber.String()
	}
	return reflect.DeepEqual(left, right)
}

func diffJSONPointers(left, right interface{}) []string {
	paths := make([]string, 0)
	collectJSONDiffPointers(left, right, "", &paths)
	if len(paths) > maxDashboardDiffPaths {
		paths = append(paths[:maxDashboardDiffPaths], "/...")
	}
	return paths
}

func collectJSONDiffPointers(left, right interface{}, prefix string, paths *[]string) {
	if len(*paths) > maxDashboardDiffPaths {
		return
	}
	leftMap, leftIsMap := left.(map[string]interface{})
	rightMap, rightIsMap := right.(map[string]interface{})
	if leftIsMap && rightIsMap {
		keys := make(map[string]struct{}, len(leftMap)+len(rightMap))
		for key := range leftMap {
			keys[key] = struct{}{}
		}
		for key := range rightMap {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			leftValue, leftExists := leftMap[key]
			rightValue, rightExists := rightMap[key]
			child := prefix + "/" + escapeJSONPointer(key)
			if !leftExists || !rightExists {
				*paths = append(*paths, child)
				continue
			}
			collectJSONDiffPointers(leftValue, rightValue, child, paths)
		}
		return
	}
	leftSlice, leftIsSlice := left.([]interface{})
	rightSlice, rightIsSlice := right.([]interface{})
	if leftIsSlice && rightIsSlice {
		if len(leftSlice) != len(rightSlice) {
			*paths = append(*paths, prefix)
			return
		}
		for index := range leftSlice {
			collectJSONDiffPointers(leftSlice[index], rightSlice[index], prefix+"/"+strconv.Itoa(index), paths)
		}
		return
	}
	if !jsonValuesEqual(left, right) {
		if prefix == "" {
			prefix = "/"
		}
		*paths = append(*paths, prefix)
	}
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func normalizedDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
