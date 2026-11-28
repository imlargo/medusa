// Command api runs the Medusa HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/imlargo/medusa/app/cmd/api/bootstrap"
)

// appName identifies the application in logs and in the startup banner.
const appName = "medusa-api"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

// run owns the application lifecycle. It exists so that shutdown always runs:
// os.Exit — and therefore log.Fatal — would skip every deferred call.
func run(ctx context.Context) (err error) {
	app, appErr := bootstrap.New(ctx, appName)
	if appErr != nil {
		return appErr
	}

	defer func() {
		err = errors.Join(err, app.Close())
	}()

	return app.Run(ctx)
}
