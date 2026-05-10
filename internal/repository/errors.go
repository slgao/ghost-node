package repository

import "errors"

var (
	ErrNotFound    = errors.New("record not found")
	ErrDuplicate   = errors.New("duplicate record")
	ErrConstraint  = errors.New("constraint violation")
)
