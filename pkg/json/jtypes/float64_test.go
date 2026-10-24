package jtypes

import (
	"encoding/json"
	"testing"
)

func TestFloat64_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		err    error
		expect float64
	}{
		{
			name:   "valid float",
			data:   `{"k": 2.3002}`,
			err:    nil,
			expect: 2.3002,
		},
		{
			name:   "valid string",
			data:   `{"k": "2.3"}`,
			err:    nil,
			expect: 2.3,
		},
		{
			name:   "valid null",
			data:   `{"k": null}`,
			err:    nil,
			expect: 0,
		},
		{
			name:   "valid int",
			data:   `{"k": 20}`,
			err:    nil,
			expect: 20,
		},
		{
			name:   "invalid string",
			data:   `{"k": "hello"}`,
			err:    ErrInvalidFloatSyntax,
			expect: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var dst struct {
				K Float64 `json:"k"`
			}
			if err := json.Unmarshal([]byte(test.data), &dst); err != test.err {
				t.Error(err)
			}
			if v := dst.K.Value(); v != test.expect {
				t.Errorf("expect %v, got %v", test.expect, v)
			}
		})
	}
}
