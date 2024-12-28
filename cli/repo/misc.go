package repo

import (
	"fmt"
	"strings"

	_ "embed"
)

// GetMajorVersionFromCode gets the major version from the code
func GetMajorVersionFromCode(code string) int {
	code = strings.ToLower(code)

	// Handle EL versions
	if strings.HasPrefix(code, "el") {
		var major int
		if _, err := fmt.Sscanf(code, "el%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	if strings.HasPrefix(code, "u") {
		var major int
		if _, err := fmt.Sscanf(code, "u%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	if strings.HasPrefix(code, "d") {
		var major int
		if _, err := fmt.Sscanf(code, "d%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	if strings.HasPrefix(code, "a") {
		var major int
		if _, err := fmt.Sscanf(code, "a%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	// Handle Ubuntu codenames
	switch code {
	case "focal":
		return 20
	case "jammy":
		return 22
	case "noble":
		return 24
	}

	// Handle Debian codenames
	switch code {
	case "bullseye":
		return 11
	case "bookworm":
		return 12
	case "trixie":
		return 13
	}

	return -1
}
