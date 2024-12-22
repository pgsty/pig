package pgext

import "strings"

/********************
* Tabulate Extension
********************/

// FilterByDistro returns a filter function that filters extensions by distribution
func FilterByDistro(distro string) func(*Extension) bool {
	if distro == "" || distro == "all" {
		return nil
	}
	return func(ext *Extension) bool {
		switch distro {
		case "rpm":
			return ext.RpmRepo != ""
		case "deb":
			return ext.DebRepo != ""
		case "el7", "el8", "el9":
			return ext.RpmRepo != ""
		case "d11", "d12", "u20", "u22", "u24":
			return ext.DebRepo != ""
		default:
			return true
		}
	}
}

// FilterByCategory returns a filter function that filters extensions by category
func FilterByCategory(category string) func(*Extension) bool {
	if category == "" || category == "all" {
		return func(ext *Extension) bool {
			return true
		}
	}
	cate := strings.ToUpper(category)
	return func(ext *Extension) bool {
		return ext.Category == cate
	}
}

// CombineFilters combines multiple filter functions into a single filter
func CombineFilters(filters ...func(*Extension) bool) func(*Extension) bool {
	return func(ext *Extension) bool {
		for _, filter := range filters {
			if filter != nil && !filter(ext) {
				return false
			}
		}
		return true
	}
}

// FilterExtensions returns a filtered list of extensions based on distro and category
func FilterExtensions(distro, category string) func(*Extension) bool {
	return CombineFilters(
		FilterByDistro(distro),
		FilterByCategory(category),
	)
}
