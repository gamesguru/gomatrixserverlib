package spec

import "fmt"

// HTTPError represents an HTTP error response, which may wrap an underlying native Go error.
type HTTPError struct {
	Contents     []byte
	WrappedError error
	Message      string
	Code         int
}

// Error implements the error interface.
func (e HTTPError) Error() string {
	var wrappedErrMsg string
	if e.WrappedError != nil {
		wrappedErrMsg = e.WrappedError.Error()
	}
	return fmt.Sprintf("contents=%s msg=%s code=%d wrapped=%s", e.Contents, e.Message, e.Code, wrappedErrMsg)
}

// RespError is the standard JSON error response from Matrix Homeservers.
// See http://matrix.org/docs/spec/client_server/r0.2.0.html#api-standards
type RespError struct {
	ErrCode string `json:"errcode"`
	Err     string `json:"error"`
}

// Error implements the error interface.
func (e RespError) Error() string {
	return e.ErrCode + ": " + e.Err
}
