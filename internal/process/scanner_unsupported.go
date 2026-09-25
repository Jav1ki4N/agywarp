//go:build !linux

package process

import (
	"context"
	"errors"
)

var ErrUnsupportedPlatform = errors.New("process scanner is only supported on Linux")

type UnsupportedScanner struct{}

func NewScanner() Scanner {
	return &UnsupportedScanner{}
}

func (s *UnsupportedScanner) Scan(ctx context.Context) ([]RunningProcess, error) {
	return nil, ErrUnsupportedPlatform
}
