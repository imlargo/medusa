// Command worker runs the jobs the API enqueues. Run as many replicas as the
// work needs, next to the API.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/imlargo/medusa/app/cmd/api/bootstrap"
)

const appName = "medusa-worker"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

// run exists so that Close always runs: os.Exit would skip it.
func run(ctx context.Context) (err error) {
	app, appErr := bootstrap.NewWorker(ctx, appName)
	if appErr != nil {
		return appErr
	}
	defer func() {
		err = errors.Join(err, app.Close())
	}()

	return app.Run(ctx)
}
