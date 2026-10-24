package jtypes

import (
	"http-api-template/pkg/bytesconv"
	"time"
)

const iso8601Layout = "2006-01-02T15:04:05+07:00"

type ISO8601 struct {
	time.Time
}

func (rt ISO8601) MarshalJSON() ([]byte, error) {
	return bytesconv.StringToBytes(`"` + rt.Format(iso8601Layout) + `"`), nil
}
