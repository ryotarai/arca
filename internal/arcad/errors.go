package arcad

import "errors"

var (
	ErrExposureNotFound = errors.New("exposure not found")
	ErrInvalidTicket    = errors.New("invalid ticket")
	ErrInvalidSession   = errors.New("invalid session")
)
