package jtypes

import (
	"strings"
	"time"

	"http-api-template/pkg/bytesconv"
)

type YYYYMMDDate struct {
	time.Time
}

func (rt *YYYYMMDDate) UnmarshalJSON(d []byte) (err error) {
	s := strings.Trim(bytesconv.BytesToString(d), `"`)
	if s == "null" || s == "" {
		rt.Time = time.Time{}
		return
	}
	rt.Time, err = time.Parse("2006-01-02", s)
	return
}
