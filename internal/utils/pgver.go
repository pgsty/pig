package utils

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ParsePostgresVersion will parse the major and minor version of PostgreSQL
func ParsePostgresVersion(input string) (int, int, error) {
	s := strings.TrimSpace(input)

	// 1. Find the index of the first digit in the string
	startIdx := -1
	for i, r := range s {
		if unicode.IsDigit(r) {
			startIdx = i
			break
		}
	}
	if startIdx == -1 {
		return 0, 0, fmt.Errorf("no digits found in %q", input)
	}

	// 2. Collect digits starting from startIdx to determine the major version
	//    If more than two digits are found, report an error
	endIdx := startIdx
	digitCount := 0
	for ; endIdx < len(s); endIdx++ {
		if !unicode.IsDigit(rune(s[endIdx])) {
			break
		}
		digitCount++
		if digitCount > 2 {
			return 0, 0, fmt.Errorf("the first number in %q has more than two digits, likely >= 100", input)
		}
	}

	majorStr := s[startIdx:endIdx] // Extract the first 1-2 digits
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid major version %q: %v", majorStr, err)
	}
	if major < 1 || major > 99 {
		return 0, 0, fmt.Errorf("major version %d is out of range [1..99]", major)
	}

	// 3. Check if a '.' followed by digits exists for the minor version; default to minor=0 if not
	minor := 0

	// leftover is the substring after endIdx, e.g., ".10", ".5something", "rc2", " some-other-thing"
	leftover := s[endIdx:]
	leftover = strings.TrimLeftFunc(leftover, func(r rune) bool {
		// Skip all characters until a '.' is found
		return r != '.'
	})

	// If leftover is empty or doesn't start with '.', there's no minor version
	if leftover == "" || leftover[0] != '.' {
		return major, 0, nil
	}

	// Extract potential minor version digits after '.'
	restAfterDot := leftover[1:]
	restAfterDot = strings.TrimLeftFunc(restAfterDot, func(r rune) bool {
		return !unicode.IsDigit(r)
	})
	if restAfterDot == "" {
		return major, 0, nil
	}

	// Collect digits for the minor version
	digitsForMinor := 0
	minorStrBuilder := strings.Builder{}
	for _, r := range restAfterDot {
		if !unicode.IsDigit(r) {
			break
		}
		minorStrBuilder.WriteRune(r)
		digitsForMinor++
	}
	if digitsForMinor == 0 {
		return major, 0, nil
	}

	minorStr := minorStrBuilder.String()
	m, err := strconv.Atoi(minorStr)
	if err == nil {
		minor = m
	}

	return major, minor, nil
}
