//go:build go_json
// +build go_json

package json

import (
	gojson "github.com/goccy/go-json"
)

var (
	// Marshal is exported by gin/json package.
	Marshal = gojson.Marshal
	// Unmarshal is exported by gin/json package.
	Unmarshal = gojson.Unmarshal
	// MarshalIndent is exported by gin/json package.
	MarshalIndent = gojson.MarshalIndent
	// NewDecoder is exported by gin/json package.
	NewDecoder = gojson.NewDecoder
	// NewEncoder is exported by gin/json package.
	NewEncoder = gojson.NewEncoder
)
