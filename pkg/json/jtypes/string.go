package jtypes

import (
	"http-api-template/pkg/bytesconv"
)

type String struct {
	v string
}

func (i *String) UnmarshalJSON(d []byte) error {
	if d[0] == 'n' {
		return nil
	}
	if d[0] == '"' {
		i.v = bytesconv.BytesToString(d[1 : len(d)-1])
		return nil
	}
	i.v = bytesconv.BytesToString(d)
	return nil
}

func (i *String) Value() string {
	return i.v
}
