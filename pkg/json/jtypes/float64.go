package jtypes

import (
	"errors"
	"http-api-template/pkg/json"
	"strconv"
)

var ErrInvalidFloatSyntax = errors.New("invalid syntax")

type Float64 struct {
	v float64
}

func (f *Float64) UnmarshalJSON(d []byte) error {
	var dst interface{}
	if err := json.Unmarshal(d, &dst); err != nil {
		return err
	}
	switch v := dst.(type) {
	case string:
		val, err := strconv.ParseFloat(v, 64) // nolint:gomnd
		if err != nil {
			return ErrInvalidFloatSyntax
		}
		f.v = val
	case float64:
		f.v = v
	case int:
		f.v = float64(v)
	}
	return nil
}

func (f *Float64) Value() float64 {
	return f.v
}
