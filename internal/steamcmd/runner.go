package steamcmd

import "context"

type Runner interface {
	Detect(ctx context.Context) (string, error)
	Install(ctx context.Context, installDir string) error
	SelfUpdate(ctx context.Context) error
	InstallApp(ctx context.Context, appID int, installDir string, validate bool) (<-chan string, error)
}
