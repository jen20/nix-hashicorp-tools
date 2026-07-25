package releases

import (
	"errors"
)

var (
	// ErrInvalidProduct indicates that a product name supplied as a parameter is invalid.
	// This is usually because the value is empty.
	ErrInvalidProduct = errors.New("invalid product")

	// ErrInvalidLicenseClass indicates that a license class supplied as a parameter is
	// invalid. Note that package-level instance variables are provided for all valid license
	// classes, which may be used instead of string values.
	ErrInvalidLicenseClass = errors.New("invalid license class")

	// ErrConstructingRequest indicates http.NewRequestWithContext fails. The cause is
	// wrapped.
	ErrConstructingRequest = errors.New("failed to construct HTTP request")

	// ErrInvalidResponseContentType indicates that the server returned an undocumented
	// content type.
	ErrInvalidResponseContentType = errors.New("invalid response content type")

	// ErrInvalidResponseBody indicates that the server returned an undocumented object
	// structure.
	ErrInvalidResponseBody = errors.New("invalid response body")

	// ErrInvalidStatusCode indicates that the server returned a status code other than "200 OK".
	ErrInvalidStatusCode = errors.New("invalid response status code")
)
