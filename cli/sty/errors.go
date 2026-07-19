package sty

import (
	"errors"
	"fmt"
)

// commandError gives legacy text executors stable structured status codes
// without coupling the Cobra layer to implementation-specific error types.
type commandError struct {
	code int
	err  error
}

func (e *commandError) Error() string   { return e.err.Error() }
func (e *commandError) Unwrap() error   { return e.err }
func (e *commandError) StatusCode() int { return e.code }

func commandFailed(code int, err error) error {
	if err == nil {
		return nil
	}
	var coded interface{ StatusCode() int }
	if errors.As(err, &coded) {
		return err
	}
	return &commandError{code: code, err: err}
}

func commandFailedf(code int, format string, args ...interface{}) error {
	return commandFailed(code, fmt.Errorf(format, args...))
}
