package rpc

import "strings"

// QuoteCommandValue renders v as a double-quoted string literal for rtorrent's
// command grammar (daemon rpc/parse.cc): inside quotes only '\' escapes and '"'
// closes, while ',' / ';' / whitespace lose their separator meaning. Trailing
// load.* params are re-parsed by the daemon as full commands, so any
// user-controlled value embedded in one MUST pass through here.
//
// Quoting cannot neutralize command substitution: rtorrent EXECUTES any parsed
// argument string whose first character is '$' (parse_command_execute), after
// quotes are stripped. Callers must reject values with a leading '$' outright.
func QuoteCommandValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}
