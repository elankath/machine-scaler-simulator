package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/gardenclient"
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

	gardenShootName := os.Getenv("GARDEN_SHOOT_NAME")
	if len(gardenShootName) == 0 {
		slog.Error("GARDEN_SHOOT_NAME env must be set")
		os.Exit(1)
	}

	shootAccess, err := gardenclient.InitShootAccess(gardenProjectName, gardenShootName)
	if err != nil {
		slog.Error("cannot initialize shoot access", "error", err)
		os.Exit(2)
		return
	}
	shootObj, err := shootAccess.GetShootObj()
	if err != nil {
		slog.Error("cannot get shoot obj", "project-name", gardenProjectName, "shoot-name", gardenShootName, "error", err)
		os.Exit(3)
		return
	}
	slog.Info("retrieved shoot obj.", "provider", shootObj.Spec.Provider.Type, "num-workerpool", len(shootObj.Spec.Provider.Workers))

	shootNodes, err := shootAccess.GetNodes()
	if err != nil {
		slog.Error("cannot get shoot nodes", "project-name", gardenProjectName, "shoot-name", gardenShootName, "error", err)
		os.Exit(4)
		return
	}
	slog.Info("retrieved shoot nodes", "num-nodes", len(shootNodes))

	// TODO: uncomment below lines. They are commented because otherwise there are complains that the var is not being used
	// var model gardenclient.Model
	// model = *gardenclient.Connect()

	//virtualClusterAccess, err := virtualclient.InitializeAccess(scheme.Scheme, binaryAssetsDir, map[string]string{
	//	//		"secure-port": apiServerPort, <--TODO: this DOESN'T work..ask maddy on envtest port config
	//})
	//if err != nil {
	//	slog.Error("cannot initialize virtual cluster", "error", err)
	//	os.Exit(2)
	//}
	//slog.Info("INITIALIZATION COMPLETE!")
	//waitForSignalAndShutdown(virtualClusterAccess)
	// wait the signal
}

func waitForSignalAndShutdown(virtualAccess scalesim.VirtualClusterAccess) {
	slog.Info("Waiting until quit...")
	quit := make(chan os.Signal, 1)

	/// Use signal.Notify() to listen for incoming SIGINT and SIGTERM signals and relay them to the quit channel.
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	s := <-quit
	slog.Warn("Cleanup and Exit!", "signal", s.String())
	err := virtualAccess.Shutdown()
	if err != nil {
		slog.Warn("error in shutdown", "error", err)
		os.Exit(10)
	}

}
