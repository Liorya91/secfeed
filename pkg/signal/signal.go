package signal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

var onlyOneSignalHandler = make(chan struct{})

// SetupHandler registers for SIGTERM and SIGINT. A context is returned
// which is canceled on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
// It also returns a cancel func that can be used to shut down the context
// manually.
func SetupHandler() (context.Context, context.CancelFunc) {
	close(onlyOneSignalHandler) // panics when closed twice

	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-c
		cancel()
		<-c
		os.Exit(1) // Second signal. Exit immediately.
	}()

	return ctx, cancel
}
