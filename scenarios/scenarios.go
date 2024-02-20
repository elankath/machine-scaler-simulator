package scenarios

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/elankath/scaler-simulator/serutil"
	"github.com/elankath/scaler-simulator/webutil"

	scalesim "github.com/elankath/scaler-simulator"
)

// One worker pool, min=1, max=2
// 1 node up
// Add multiple pods. Should assign some pods to the running node, and recommand scaling up a node for the remaining
func NewScenarioAOld(ctx context.Context, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess, w http.ResponseWriter, podsCount int) {
	fmt.Fprintln(w, "Executing scenario A")

	SyncNodesInShoot(ctx, virtualAccess, shootAccess, w)
	if err := virtualAccess.RemoveTaintFromNode(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	objects, err := serutil.ConstructK8sObject(GetYamlFilePath("pod.yaml"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i := 0; i < podsCount; i++ {
		err = virtualAccess.ApplyK8sObject(ctx, objects...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

func NewScenarioB(ctx context.Context, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess, w http.ResponseWriter, podsCount int) {
	webutil.Log(w, "Executing scenario B")

	SyncNodesInShoot(ctx, virtualAccess, shootAccess, w)
	if err := virtualAccess.RemoveTaintFromNode(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pod, err := serutil.ReadPod(GetYamlFilePath("scenarioB_pods.yaml"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i := 0; i < podsCount; i++ {
		slog.Info("Creating pod", "pod", pod.Name, "namespace", pod.Namespace)
		err = virtualAccess.CreatePods(ctx, pod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
	webutil.Log(w, "Done Execution of scenario B")
}

func NewScenarioC(ctx context.Context, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess, w http.ResponseWriter, podsACount, podsBCount int) {
	webutil.Log(w, "Executing scenario C")

	SyncNodesInShoot(ctx, virtualAccess, shootAccess, w)
	if err := virtualAccess.RemoveTaintFromNode(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	podA, err := serutil.ReadPod(GetYamlFilePath("scenarioC_podA.yaml"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	podB, err := serutil.ReadPod(GetYamlFilePath("scenarioC_podB.yaml"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i := 0; i < podsACount; i++ {
		slog.Info("Creating pod", "pod", podA.Name, "namespace", podA.Namespace)
		err := virtualAccess.CreatePods(ctx, podA)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for i := 0; i < podsBCount; i++ {
		slog.Info("Creating pod", "pod", podA.Name, "namespace", podB.Namespace)
		err := virtualAccess.CreatePods(ctx, podB)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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

		for _, worker := range shoot.Spec.Provider.Workers {
			nodeCreated, err := virtualAccess.CreateNodeInWorkerGroup(ctx, &worker)
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
	}
	slog.Log(ctx, slog.LevelInfo, "No unscheduled pods present. Finishing scenario C", "num-nodes-created", TotalNodesCreated)
	webutil.Log(w, "Done Execution of scenario C")
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
