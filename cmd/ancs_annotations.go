package cmd

import "strconv"

// ancsAnn builds the 9 required ANCS annotations in a consistent way.
// Keep this tiny: cmd layer should stay declarative.
func ancsAnn(name, typ, volatility, parallel string, idempotent bool, risk, confirm, osUser string, cost int) map[string]string {
	return map[string]string{
		"name":       name,
		"type":       typ,
		"volatility": volatility,
		"parallel":   parallel,
		"idempotent": strconv.FormatBool(idempotent),
		"risk":       risk,
		"confirm":    confirm,
		"os_user":    osUser,
		"cost":       strconv.Itoa(cost),
	}
}

func mergeAnn(base map[string]string, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}
