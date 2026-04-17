package cli

import "strings"

// hamsFlagFalsey reports whether a `--hams-<flag>=<value>` value parses
// as false-y (so the parser can elide the key from the resulting map
// entirely). Without this, `--hams-local=false` would add the "local"
// key with value "false"; presence-check call sites
// (`if _, ok := hamsFlags["local"]; ok`) would interpret it as true,
// surprising users who expected `=false` to disable the flag.
//
// Recognized false-y values: "false" and "0" (case-insensitive). All
// other values (including "" / bare flag) are treated as truthy and
// kept in the map. Boolean-like provider flags (`--hams-local`,
// `--hams-lucky`) get this guard for free without per-call-site code
// changes — the 14 existing presence-check sites stay correct.
func hamsFlagFalsey(value string) bool {
	switch strings.ToLower(value) {
	case "false", "0":
		return true
	}
	return false
}

const hamsFlagPrefix = "--hams-"

// splitHamsFlags separates --hams- prefixed flags from passthrough args.
// Called by routeToProvider before dispatching to the provider handler.
// Also handles the -- separator: everything after -- goes to passthrough.
func splitHamsFlags(args []string) (hamsFlags map[string]string, passthrough []string) {
	hamsFlags = make(map[string]string)
	forceForward := false

	for _, arg := range args {
		if forceForward {
			passthrough = append(passthrough, arg)
			continue
		}

		if arg == "--" {
			forceForward = true
			passthrough = append(passthrough, arg)
			continue
		}

		if strings.HasPrefix(arg, hamsFlagPrefix) {
			key, value := parseHamsFlag(arg[len(hamsFlagPrefix):])
			if hamsFlagFalsey(value) {
				// Cycle 162: explicit false-y values disable the flag.
				continue
			}
			hamsFlags[key] = value
			continue
		}

		passthrough = append(passthrough, arg)
	}

	return hamsFlags, passthrough
}

func parseHamsFlag(s string) (key, value string) {
	if k, v, ok := strings.Cut(s, "="); ok {
		return k, v
	}
	return s, ""
}
