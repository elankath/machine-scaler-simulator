package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
)

// type scenarios struct {
// 	engine scalesim.Engine
// }

// One worker pool, min=1, max=2
// 1 node up
// Add multiple pods. Should assign some pods to the running node, and recommand scaling up a node for the remaining
func NewScenarioA(ctx context.Context, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess, w http.ResponseWriter) {
	fmt.Fprintln(w, "Executing scenario A")

	SyncNodesInShoot(ctx, virtualAccess, shootAccess, w)
	if err := virtualAccess.RemoveTaintFromNode(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	objects, err := shootAccess.ConstructK8sObject(GetYamlFilePath("scenarioA_objects.yaml"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = virtualAccess.ApplyK8sObject(ctx, objects)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Log(ctx, slog.LevelInfo, "Waiting for 15 seconds to give scheduler some breathing space")
	<-time.After(15 * time.Second)

	TotalNodesCreated := 0
	shoot, err := shootAccess.GetShootObj()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for {
		eventList, err := virtualAccess.GetFailedSchedulingEvents(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(eventList) == 0 {
			slog.Log(ctx, slog.LevelInfo, "No FailedScheduling events present, exiting...")
			break
		}
		slog.Log(ctx, slog.LevelInfo, "Unscheduled pods present. Creating a new node to schedule these pods", "num-pending-pods", len(eventList))

		nodeCreated, err := virtualAccess.CreateNodeInWorkerGroup(ctx, &shoot.Spec.Provider.Workers[0])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !nodeCreated {
			slog.Log(ctx, slog.LevelError, "error creating node. Node could not be created. WorkerGroup max reached")
			break
		}
		if err := virtualAccess.RemoveTaintFromNode(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		TotalNodesCreated = TotalNodesCreated + 1

		slog.Log(ctx, slog.LevelInfo, "Node created. Waiting for 15 seconds and retrying...")
		<-time.After(15 * time.Second)
	}
	slog.Log(ctx, slog.LevelInfo, "No unscheduled pods present. Finishing scenario A", "num-nodes-created", TotalNodesCreated)
	fmt.Fprintln(w, "Done Execution of scenario A")
}

func SyncNodesInShoot(ctx context.Context, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess, w http.ResponseWriter) {
	nodes, err := shootAccess.GetNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = virtualAccess.AddNodes(ctx, nodes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("added nodes to virtual cluster.", "num-nodes", len(nodes))
}

func GetYamlFilePath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		slog.Error("no working dir found", "error", err)
		os.Exit(1)
	}
	return wd + "/scenarios/" + path
}
