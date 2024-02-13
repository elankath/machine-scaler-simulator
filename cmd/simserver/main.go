package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/elankath/scaler-simulator/habitat"
)

func main() {
	binaryAssetsDir := os.Getenv("BINARY_ASSETS_DIR")
	if len(binaryAssetsDir) == 0 {
		slog.Error("BINARY_ASSETS_DIR env must be set to a dir path containing binaries")
		os.Exit(1)
	}

	//apiServerPort := "50000" // TODO: put this in env too
	habitatAccess, err := habitat.InitHabitat(scheme.Scheme, binaryAssetsDir, map[string]string{
		//		"secure-port": apiServerPort,
	})
	if err != nil {
		slog.Error("error initializing habtat", "error", err)
		os.Exit(2)
	}
	slog.Info("INITIALIZATION COMPLETE!")
	// wait the signal
	slog.Info("Waiting until quit...")
	quit := make(chan os.Signal, 1)

	/// Use signal.Notify() to listen for incoming SIGINT and SIGTERM signals and relay them to the quit channel.
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	s := <-quit
	slog.Warn("Cleanup and Exit!", "signal", s.String())
	err = habitatAccess.Environment.Stop()
	if err != nil {
		slog.Warn("error stopping environment", "error", err)
		os.Exit(10)
	}
}
