package jtypes

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name   string
		expect string
		arg    interface{}
	}{
		{name: "null", expect: "", arg: "null"},
		{name: "string", expect: "hello world ", arg: `"hello world "`},
		{name: "float", expect: "10.22", arg: 10.22},
		{name: "int", expect: "656006", arg: 656006},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v String
			switch arg := test.arg.(type) {
			case int:
				dst := make([]byte, 0)
				dst = strconv.AppendInt(dst, int64(arg), 10)
				if err := json.Unmarshal(dst, &v); err != nil {
					t.Error(err)
				}
			case float64:
				dst := make([]byte, 0)
				dst = strconv.AppendFloat(dst, arg, 'g', -1, 64)
				if err := json.Unmarshal(dst, &v); err != nil {
					t.Error(err)
				}
			case string:
				if err := json.Unmarshal([]byte(arg), &v); err != nil {
					t.Error(err)
				}
			}
			if v.Value() != test.expect {
				t.Fatalf("expect: %q, got: %q", test.expect, v.Value())
			}
		})
	}
}
