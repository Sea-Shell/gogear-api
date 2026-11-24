package models

import (
	"bytes"
	"fmt"
	"math/big"
)

// WeightTotal is the on-your-back weight of a loadout in grams (BR-029 AC-3):
// an exact integer with no upper bound. Every quantity and weight the API
// accepts is kept, and their products and sum can pass 2^63-1, so the total is
// not an int64. It is written to JSON as a bare integer, never quoted and
// never a float, and read back from JSON only in that form. The zero value is
// 0. Clients that parse JSON numbers as float64 (JavaScript) round a total
// above 2^53; the wire value is exact.
//
// Swagger sees an integer with no format, which means unbounded.
type WeightTotal struct{ n *big.Int }

// NewWeightTotal takes ownership of n; nil is 0.
func NewWeightTotal(n *big.Int) WeightTotal { return WeightTotal{n} }

// String is the decimal integer, as written to JSON.
func (w WeightTotal) String() string {
	if w.n == nil {
		return "0"
	}
	return w.n.String()
}

// MarshalJSON writes the bare decimal integer. It has a value receiver so a
// WeightTotal field and a pointer to one both encode this way.
func (w WeightTotal) MarshalJSON() ([]byte, error) { return []byte(w.String()), nil }

// UnmarshalJSON accepts only an unquoted integer literal: no string, null,
// fraction or exponent, which a total never has.
func (w *WeightTotal) UnmarshalJSON(b []byte) error {
	if !isIntegerLiteral(b) {
		return fmt.Errorf("total weight %q is not an integer literal", b)
	}
	n, ok := new(big.Int).SetString(string(b), 10)
	if !ok {
		return fmt.Errorf("total weight %q is not an integer literal", b)
	}
	w.n = n
	return nil
}

// isIntegerLiteral reports whether b is an optional minus sign and digits
// without a leading zero (JSON's integer form).
func isIntegerLiteral(b []byte) bool {
	b = bytes.TrimPrefix(b, []byte("-"))
	if len(b) == 0 || (b[0] == '0' && len(b) > 1) {
		return false
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
