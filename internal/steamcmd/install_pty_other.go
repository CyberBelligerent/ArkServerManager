//go:build !windows

package steamcmd

import (
	"context"
	"errors"
)

func (r *RealRunner) installAppPTY(_ context.Context, _ int, _ string, _ bool) (<-chan string, error) {
	return nil, errors.New("ConPTY is Windows-only")
}
