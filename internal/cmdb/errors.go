package cmdb

import "errors"

// Error is a value-free CMDB adapter failure. It intentionally does not wrap
// driver errors because those can contain connection strings, SQL, or values.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return "CMDB_ERROR"
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// IsCode reports whether err belongs to one stable, value-free CMDB family.
func IsCode(err error, code string) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

func failure(code, message string) error {
	return &Error{Code: code, Message: message}
}
