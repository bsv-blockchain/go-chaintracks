package chaintracks

import (
	"fmt"
	"strconv"
)

// ParseHeightAndCount parses height and count string parameters for multi-header endpoints.
func ParseHeightAndCount(heightStr, countStr string) (uint32, uint32, error) {
	if heightStr == "" || countStr == "" {
		return 0, 0, fmt.Errorf("%w: height or count", ErrMissingParameter)
	}
	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: height", ErrInvalidParameter)
	}
	count, err := strconv.ParseUint(countStr, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: count", ErrInvalidParameter)
	}
	return uint32(height), uint32(count), nil
}
