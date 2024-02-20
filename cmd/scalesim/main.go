package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes/scheme"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/engine"
	"github.com/elankath/scaler-simulator/virtualcluster"
)

func main() {
	binaryAssetsDir := os.Getenv("BINARY_ASSETS_DIR")
	if len(binaryAssetsDir) == 0 {
		slog.Error("BINARY_ASSETS_DIR env must be set to a dir path containing binaries")
		os.Exit(1)
	}

	gardenProjectName := os.Getenv("GARDEN_PROJECT_NAME")
	if len(gardenProjectName) == 0 {
		slog.Error("GARDEN_PROJECT_NAME env must be set")
		os.Exit(1)
	}

	virtualClusterAccess, err := virtualcluster.InitializeAccess(scheme.Scheme, binaryAssetsDir, map[string]string{
		//		"secure-port": apiServerPort, <--TODO: this DOESN'T work..ask maddy on envtest port config
	})
	if err != nil {
		slog.Error("cannot initialize virtual cluster", "error", err)
		os.Exit(3)
	}

	eng, err := engine.NewEngine(virtualClusterAccess, gardenProjectName)
	if err != nil {
		slog.Error("cannot initialize simulator engine", "error", err)
		os.Exit(4)
	}

	httpServer := &http.Server{
		Addr:    net.JoinHostPort("localhost", "8080"),
		Handler: eng,
	}

	time.Sleep(3 * time.Second)
	slog.Info("INITIALIZATION COMPLETE!!", "engine-addr", httpServer.Addr)

	go waitForSignalAndShutdown(virtualClusterAccess, httpServer)

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("engine cannot listen/serve, SHUTTING DOWN.", "error", err)
		err := syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		if err != nil {
			slog.Error("cannot term self!", "error", err)
		}
	}
}

func waitForSignalAndShutdown(virtualAccess scalesim.VirtualClusterAccess, httpServer *http.Server) {
	slog.Info("Waiting until quit...")
	quit := make(chan os.Signal, 1)

	/// Use signal.Notify() to listen for incoming SIGINT and SIGTERM signals and relay them to the quit channel.
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	s := <-quit
	slog.Warn("Cleanup and Exit!", "signal", s.String())
	virtualAccess.Shutdown()

	if err := httpServer.Shutdown(context.Background()); err != nil {
		slog.Error("cannot shut down http engine", "error", err)
	}
}
