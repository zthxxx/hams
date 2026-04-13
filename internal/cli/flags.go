package cli

import "strings"

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
