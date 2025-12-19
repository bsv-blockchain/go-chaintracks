// Package chaintracks provides core types and interfaces for blockchain header tracking.
package chaintracks

import "errors"

var (
	// ErrHeaderNotFound is returned when a header cannot be found
	ErrHeaderNotFound = errors.New("header not found")

	// ErrDuplicateHeader is returned when trying to add a header that already exists
	ErrDuplicateHeader = errors.New("duplicate header")

	// ErrInvalidHeader is returned when a header fails validation
	ErrInvalidHeader = errors.New("invalid header")

	// ErrInsufficientPoW is returned when a header doesn't meet the difficulty target
	ErrInsufficientPoW = errors.New("insufficient proof of work")

	// ErrBrokenChain is returned when a header's previous hash doesn't link to known chain
	ErrBrokenChain = errors.New("broken chain linkage")

	// ErrInvalidTimestamp is returned when a header has an invalid timestamp
	ErrInvalidTimestamp = errors.New("invalid timestamp")

	// ErrSSEStreamFailed is returned when SSE stream returns non-200 status
	ErrSSEStreamFailed = errors.New("SSE stream failed")

	// ErrServerRequestFailed is returned when a server request fails
	ErrServerRequestFailed = errors.New("server request failed")

	// ErrServerReturnedError is returned when server returns an error status
	ErrServerReturnedError = errors.New("server returned error status")

	// ErrInvalidFileSize is returned when file size is invalid
	ErrInvalidFileSize = errors.New("invalid file size")

	// ErrP2PAlreadyStarted is returned when P2P is already running
	ErrP2PAlreadyStarted = errors.New("P2P already started")

	// ErrInvalidHeaderSize is returned when header size is invalid
	ErrInvalidHeaderSize = errors.New("invalid header size")

	// ErrNoHeadersReturned is returned when no headers are returned from server
	ErrNoHeadersReturned = errors.New("no headers returned, cannot find common ancestor")

	// ErrCommonAncestorNotFound is returned when common ancestor cannot be found
	ErrCommonAncestorNotFound = errors.New("could not find common ancestor")

	// ErrBestBlockHeaderFailed is returned when bestblockheader endpoint fails
	ErrBestBlockHeaderFailed = errors.New("bestblockheader endpoint failed")

	// ErrInvalidHeaderDataLength is returned when header data length is invalid
	ErrInvalidHeaderDataLength = errors.New("invalid header data length")

	// ErrIntegerOverflow is returned when an integer overflow would occur
	ErrIntegerOverflow = errors.New("integer overflow in conversion")

	// ErrP2PClientRequired is returned when P2P client is required but not provided
	ErrP2PClientRequired = errors.New("p2pClient is required")

	// ErrUnknownNetwork is returned when an unknown network is specified
	ErrUnknownNetwork = errors.New("unknown network")

	// ErrChaintracksURLRequired is returned when chaintracks URL is required for remote mode
	ErrChaintracksURLRequired = errors.New("chaintracks URL required for remote mode")

	// ErrUnknownChaintracksMode is returned when an unknown chaintracks mode is specified
	ErrUnknownChaintracksMode = errors.New("unknown chaintracks mode")

	// ErrInvalidResponseLength is returned when response length is invalid
	ErrInvalidResponseLength = errors.New("invalid response length")
)
