package endpoints

import "strings"

// emailWhitespace lists every rune strings.TrimSpace (unicode.IsSpace) strips.
// SQLite's trim() only strips what it is given, so the SQL side of an email
// comparison passes this set to match the Go side exactly (BR-004 AC-4).
const emailWhitespace = "\t\n\v\f\r \u0085  " +
	"           " +
	"    　"

// normalizeEmail is the stored and compared form of an email: surrounding
// whitespace removed, case kept (P-7).
func normalizeEmail(s string) string { return strings.TrimSpace(s) }
