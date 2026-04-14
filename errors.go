package tn3270

import (
	"errors"
	"fmt"
)

// Error codes scripts can match on via err.code at the JS boundary (sobek
// exposes exported struct fields verbatim, so Code/Message must stay PascalCase).
const (
	CodeInvalidArgument  = "invalid_argument"
	CodeNotConnected     = "not_connected"
	CodeConnectFailed    = "connect_failed"
	CodeConnectTimeout   = "connect_timeout"
	CodeTLSHandshake     = "tls_handshake"
	CodeNegotiation      = "negotiation_failed"
	CodeWaitTimeout      = "wait_timeout"
	CodeHostClosed       = "host_closed"
	CodeProtocol         = "protocol_error"
	CodeInitContext      = "init_context"
	CodeProtectedField   = "protected_field"
	CodeScreenshotFailed = "screenshot_failed"
)

type Tn3270Error struct {
	Code    string
	Message string
	cause   error
}

func (e *Tn3270Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Tn3270Error) Unwrap() error { return e.cause }

func newError(code, message string) *Tn3270Error {
	return &Tn3270Error{Code: code, Message: message}
}

func wrapError(code, message string, cause error) *Tn3270Error {
	return &Tn3270Error{Code: code, Message: message, cause: cause}
}

func asCode(err error) string {
	var te *Tn3270Error
	if errors.As(err, &te) {
		return te.Code
	}
	return ""
}
