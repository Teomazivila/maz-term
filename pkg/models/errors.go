package models

import "errors"

// ErrNotFound is returned when an operation targets a record that does not
// exist. It lives here rather than in the storage package so that the UI can
// distinguish "no such record" from an infrastructure failure without importing
// the storage implementation.
var ErrNotFound = errors.New("not found")
