package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/elankath/scaler-simulator/scenarios/d"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"

	"github.com/elankath/scaler-simulator/gardenclient"
	"github.com/elankath/scaler-simulator/scenarios/a"
	"github.com/elankath/scaler-simulator/scenarios/c"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"

	scalesim "github.com/elankath/scaler-simulator"
)

type engine struct {
	virtualAccess       scalesim.VirtualClusterAccess
	mux                 *http.ServeMux
	mu                  sync.Mutex
	shootAccessMap      map[string]scalesim.ShootAccess
	gardenLandscapeName string
	gardenProjectName   string
}

var _ scalesim.Engine = (*engine)(nil)

func NewEngine(virtualAccess scalesim.VirtualClusterAccess, gardenLandscapeName string, gardenProjectName string) (scalesim.Engine, error) {
	mux := http.NewServeMux()

	engine := &engine{
		virtualAccess:       virtualAccess,
		mux:                 mux,
		gardenLandscapeName: gardenLandscapeName,
		gardenProjectName:   gardenProjectName,
		shootAccessMap:      make(map[string]scalesim.ShootAccess),
	}
	engine.addRoutes()
	return engine, nil
}

func (e *engine) ShootAccess(shootName string) scalesim.ShootAccess {
	e.mu.Lock()
	defer e.mu.Unlock()
	access, ok := e.shootAccessMap[shootName]
	if !ok {
		access = gardenclient.InitShootAccess(e.gardenLandscapeName, e.gardenProjectName, shootName)
		e.shootAccessMap[shootName] = access
	}
	return access
}

func (e *engine) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	e.mux.ServeHTTP(writer, request)
}

func (e *engine) VirtualClusterAccess() scalesim.VirtualClusterAccess {
	return e.virtualAccess
}

func (e *engine) addRoutes() {
	e.mux.Handle("DELETE /op/virtual-cluster", e.handleClearVirtualCluster())
	e.mux.Handle("POST /op/sync/{shootName}", e.handleSyncShootNodes())
	//mux.Handle("POST /scenario/{id}/{podCount}", handleScenarios(virtualAccess, shootAccess))

	scenarioA := a.New(e)
	e.mux.Handle("POST /scenarios/"+scenarioA.Name(), scenarioA)

	scenarioC := c.New(e)
	e.mux.Handle("POST /scenarios/"+scenarioC.Name(), scenarioC)

	scenarioD := d.New(e)
	e.mux.Handle("POST /scenarios/"+scenarioD.Name(), scenarioD)

}

//func handleScenarioC(virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) http.Handler {
//	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		webutil.SetupSSEWriter(w)
//		podCountAstr := r.URL.Query().Get("podCountA")
//		podCountBstr := r.URL.Query().Get("podCountB")
//		podCountA, err := strconv.Atoi(podCountAstr)
//		if err != nil {
//			slog.Error("Error converting podCountA to int", "error", err)
//			return
//		}
//		podCountB, err := strconv.Atoi(podCountBstr)
//		if err != nil {
//			slog.Error("Error converting podCountB to int", "error", err)
//			return
//		}
//		webutil.Log(w, fmt.Sprintf("handling scenario C\n"))
//
//		scenarios.NewScenarioC(r.Context(), virtualAccess, shootAccess, w, podCountA, podCountB)
//	})
//}

func (e *engine) handleSyncShootNodes() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			webutil.SetupSSEWriter(w)
			shootName := r.PathValue("shootName")
			if shootName == "" {
				webutil.HandleShootNameMissing(w)
				return
			}
			webutil.Log(w, "Syncing nodes for shoot: "+shootName+" ...")
			err := e.SyncVirtualNodesWithShoot(r.Context(), shootName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		},
	)
}

func (e *engine) SyncVirtualNodesWithShoot(ctx context.Context, shootName string) error {
	shootAccess := e.ShootAccess(shootName)
	nodes, err := shootAccess.GetNodes()
	if err != nil {
		slog.Error("cannot get nodes from shoot.", "shoot", shootName, "error", err)
		return err
	}

	for _, node := range nodes {
		node.Labels["app.kubernetes.io/existing-node"] = "true"
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    "app.kubernetes.io/existing-node-no-schedule",
			Value:  "NoSchedule",
			Effect: corev1.TaintEffectNoSchedule,
		})
	}

	err = e.VirtualClusterAccess().AddNodes(ctx, nodes...)
	if err != nil {
		slog.Error("cannot add nodes to virtual-cluster.", "error", err)
		return err
	}
	//err = e.VirtualClusterAccess().RemoveTaintFromVirtualNodes(ctx)
	//if err != nil {
	//	slog.Error("cannot un-taint node(s).", "error", err)
	//	return err
	//}
	slog.Info("added nodes to virtual cluster.", "num-nodes", len(nodes))
	return nil
}

func (e *engine) handleClearVirtualCluster() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			webutil.SetupSSEWriter(w)
			err := e.virtualAccess.ClearAll(r.Context())
			_, _ = fmt.Fprintf(w, "Cleared virtual cluster objects")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		},
	)
}

func (e *engine) ScaleWorkerPoolsTillMaxOrNoUnscheduledPods(ctx context.Context, scenarioName string, since time.Time, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error) {
	totalNodesCreated := 0
	waitSecs := 5
	for {
		eventList, err := simutil.GetFailedSchedulingEvents(ctx, e.virtualAccess, since)
		if err != nil {
			webutil.InternalError(w, err)
			return totalNodesCreated, err
		}
		if len(eventList) == 0 {
			slog.Log(ctx, slog.LevelInfo, "No FailedScheduling events present, exiting...")
			break
		}
		slog.Log(ctx, slog.LevelInfo, "Unscheduled pods present. Creating a new node to schedule these pods", "num-pending-pods", len(eventList))
		webutil.Log(w, fmt.Sprintf("%d Unscheduled pods present. Creating a new node to schedule these pods", len(eventList)))

		for _, pool := range shoot.Spec.Provider.Workers {
			nodeCreated, err := simutil.CreateNodeInWorkerGroup(ctx, e.virtualAccess, &pool)
			if err != nil {
				webutil.InternalError(w, err)
				return totalNodesCreated, err
			}
			if !nodeCreated {
				err = errors.New("node could not be created - pool pool max reached")
				slog.Error("error creating node. ", "error", err)
				webutil.InternalError(w, err)
				return totalNodesCreated, err
			}
			if err := e.virtualAccess.RemoveTaintFromVirtualNodes(ctx); err != nil {
				webutil.InternalError(w, err)
				return totalNodesCreated, err
			}
			totalNodesCreated += 1
		}
		slog.Log(ctx, slog.LevelInfo, "+1 Nodes of worker pools created. Waiting and retrying.", "waitSecs", waitSecs)
		<-time.After(10 * time.Second)
	}
	slog.Log(ctx, slog.LevelInfo, "No unscheduled pods present.", "scenario", scenarioName, "num-nodes-created", totalNodesCreated)
	webutil.Log(w, "No Unscheduled pods present for scenario: "+scenarioName)
	return totalNodesCreated, nil
}

func (e *engine) ScaleAllWorkerPoolsTillMax(ctx context.Context, scenarioName string, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error) {
	webutil.Log(w, "Scaling virtual cluster worker pools till max for scenario: "+scenarioName)
	totalNodesCreated := 0
	for _, pool := range shoot.Spec.Provider.Workers {
		//e.virtualAccess.ListNodes(ctx)
		err := simutil.CreateNodesTillPoolMax(ctx, e.virtualAccess, &pool)
		if err != nil {
			return totalNodesCreated, err
		}
		webutil.Log(w, fmt.Sprintf("Created virtual nodes in pool: %q till max: %d", pool.Name, pool.Maximum))
		if err := e.virtualAccess.RemoveTaintFromVirtualNodes(ctx); err != nil {
			return totalNodesCreated, err
		}
		totalNodesCreated += int(pool.Maximum)
	}
	return totalNodesCreated, nil
}

func (e *engine) ScaleWorkerPoolsTillNumZonesMultPoolsMax(ctx context.Context, scenarioName string, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error) {
	webutil.Log(w, "Scaling virtual cluster worker pools til zone*pool max for scenario: "+scenarioName)
	totalNodesCreated := 0
	for _, pool := range shoot.Spec.Provider.Workers {
		err := simutil.CreateNodesTillZonexPoolMax(ctx, e.virtualAccess, shoot.Spec.Region, &pool)
		if err != nil {
			return totalNodesCreated, err
		}
		webutil.Log(w, fmt.Sprintf("Created virtual nodes in pool %q till max %d", pool.Name, pool.Maximum))
		if err := e.virtualAccess.RemoveTaintFromVirtualNodes(ctx); err != nil {
			return totalNodesCreated, err
		}
		totalNodesCreated += len(pool.Zones) * (int)(pool.Maximum)
	}
	return totalNodesCreated, nil
}
