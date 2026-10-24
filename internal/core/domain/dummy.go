package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalid      = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrConflict     = errors.New("conflict")
)

type Dummy struct {
	ID   string
	Name string
	Note string
}
type Patch struct {
	Name *string
	Note *string
}

func ValidateID(id string) error {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return fmt.Errorf("%w: ID must be a UUID", ErrInvalid)
	}
	compact := strings.ReplaceAll(id, "-", "")
	if len(compact) != 32 {
		return fmt.Errorf("%w: ID must be a UUID", ErrInvalid)
	}
	if _, err := hex.DecodeString(compact); err != nil {
		return fmt.Errorf("%w: ID must be a UUID", ErrInvalid)
	}
	return nil
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	return nil
}

func (d Dummy) Validate() error {
	if err := ValidateID(d.ID); err != nil {
		return err
	}
	return ValidateName(d.Name)
}

func (p Patch) Validate() error {
	if p.Name == nil && p.Note == nil {
		return fmt.Errorf("%w: patch must include name or note", ErrInvalid)
	}
	if p.Name != nil {
		return ValidateName(*p.Name)
	}
	return nil
}
