package models

import (
	"encoding/json"
	"math/big"
	"testing"
)

func TestWeightTotalWritesABareInteger(t *testing.T) {
	huge, _ := new(big.Int).SetString("85070591730234615847396907784232501249", 10)
	for _, c := range []struct {
		name string
		v    any
		want string
	}{
		{"zero value", WeightTotal{}, `0`},
		{"nil big", NewWeightTotal(nil), `0`},
		{"small", NewWeightTotal(big.NewInt(1994)), `1994`},
		{"past int64", NewWeightTotal(huge), `85070591730234615847396907784232501249`},
		{"pointer", &WeightTotal{n: big.NewInt(7)}, `7`},
		{"in a struct", LoadoutPublic{TotalWeight: NewWeightTotal(huge)}, ""},
	} {
		got, err := json.Marshal(c.v)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.want != "" && string(got) != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
		if c.want == "" {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(got, &m); err != nil || string(m["total_weight"]) != huge.String() {
				t.Errorf("%s: total_weight = %s (%v), want %s unquoted", c.name, m["total_weight"], err, huge)
			}
		}
	}
}

func TestWeightTotalReadsOnlyIntegerLiterals(t *testing.T) {
	for _, ok := range []string{`0`, `1994`, `18446744073709551614`, `85070591730234615847396907784232501249`} {
		var w WeightTotal
		if err := json.Unmarshal([]byte(ok), &w); err != nil || w.String() != ok {
			t.Errorf("%s -> %s, %v", ok, w, err)
		}
	}
	for _, bad := range []string{`"5"`, `null`, `1.0`, `1.5`, `1e3`, `01`, `+5`, `-`, `true`, `[]`, `0x10`} {
		var w WeightTotal
		if err := json.Unmarshal([]byte(bad), &w); err == nil {
			t.Errorf("%s accepted as %s", bad, w)
		}
	}
}
