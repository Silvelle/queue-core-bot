package handler

import "strings"

// parseCommand splits a command message into its name and arguments
//
//	"/new Practice 4"           → "new", "Practice 4", true
//	"/new@queue_bot Practice 4" → "new", "Practice 4", true
//	"/new@other_bot Practice 4" → "", "", false (meant for another bot)

func parseCommand(text, botUsername string) (name, args string, ok bool) {
	rest, found := strings.CutPrefix(text, "/")
	if !found {
		return "", "", false
	}

	cmd, args, _ := strings.Cut(rest, " ")
	name, target, addressed := strings.Cut(cmd, "@")
	if addressed && !strings.EqualFold(target, botUsername) {
		return "", "", false
	}
	if name == "" {
		return "", "", false
	}
	return name, strings.TrimSpace(args), true
}
