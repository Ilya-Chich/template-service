package jtypes

import (
	"strconv"
	"strings"

	"http-api-template/pkg/json"
)

type Link struct {
	S string
}

func (l *Link) UnmarshalJSON(d []byte) error {
	var s string
	if err := json.Unmarshal(d, &s); err != nil {
		return err
	}
	l.S = s
	return nil
}

func (l *Link) GetID() int {
	index := strings.LastIndex(l.S, "-")
	if index > -1 {
		needle := l.S[index+1:]
		id, err := strconv.Atoi(needle)
		if err != nil {
			return 0
		}
		return id
	}
	return 0
}

func (l *Link) Value() string {
	return l.S
}
