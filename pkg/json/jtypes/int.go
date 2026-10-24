package jtypes

import (
	"strconv"

	"http-api-template/pkg/bytesconv"
)

type Int struct {
	v int
}

func (i *Int) UnmarshalJSON(d []byte) error {
	if d[0] == 'n' {
		return nil
	}
	if d[0] == '"' {
		d = d[1 : len(d)-1]
	}
	v, err := strconv.Atoi(bytesconv.BytesToString(d))
	if err != nil {
		return err
	}
	i.v = v
	return nil
}

func (i *Int) Value() int {
	return i.v
}
