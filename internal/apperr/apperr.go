// Package apperr defines typed business errors so handlers can map them to
// HTTP status codes without leaking internal (database) error text.
package apperr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type Kind int

const (
	KindInvalid  Kind = iota + 1 // request is well-formed but violates a rule
	KindNotFound                 // resource missing or not owned by the caller
	KindConflict                 // state conflict, e.g. duplicate label
)

// Error is safe to show to clients: Msg is written for end users.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func Invalid(msg string) error  { return &Error{Kind: KindInvalid, Msg: msg} }
func NotFound(msg string) error { return &Error{Kind: KindNotFound, Msg: msg} }
func Conflict(msg string) error { return &Error{Kind: KindConflict, Msg: msg} }

// As returns the typed error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// NotFoundOr turns a "record not found" from the database into a NotFound
// error with msg and passes every other error through unchanged.
func NotFoundOr(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotFound(msg)
	}
	return err
}

// ConflictOnUnique turns a PostgreSQL unique-violation into a Conflict error
// with msg; other errors pass through.
func ConflictOnUnique(err error, msg string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Conflict(msg)
	}
	return err
}
