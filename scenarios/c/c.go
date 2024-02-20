package c

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
)

var shootName = "scenario-c"
var scenarioName = "C"

type scenarioC struct {
	engine scalesim.Engine
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioC{
		engine: engine,
	}
}

// Scenario C will first scale up nodes in all worker pools of scenario-c shoot to MAX
// Then deploy Pods small and large according to count.
// Then wait till all Pods are scheduled or till timeout.
func (s *scenarioC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	webutil.Log(w, "Commencing scenario: "+s.Name()+"...")
	webutil.Log(w, "Clearing virtual cluster..")
	err := s.engine.VirtualClusterAccess().ClearAll(r.Context())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Synchronizing virtual nodes with nodes of shoot: %s ...", shootName))
	err = s.engine.SyncVirtualNodesWithShoot(r.Context(), shootName)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	smallCount := webutil.GetIntQueryParam(r, "small", 12) //total = 12x2=24M small
	largeCount := webutil.GetIntQueryParam(r, "large", 8)  // total = 8*7=56M large

	podSpecPath := "scenarios/c/podSmall.yaml"
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, smallCount))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, smallCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	podSpecPath = "scenarios/c/podLarge.yaml"
	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, largeCount))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, largeCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Deployed %d Pods..wait for scheduler to sync...", smallCount+largeCount))
	<-time.After(8 * time.Second)

	webutil.Log(w, "Scaling till worker pool max...")
	numCreatedNodes, err := s.engine.ScaleAllWorkerPoolsTillMax(r.Context(), s.Name(), shoot, w)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Created %d total nodes", numCreatedNodes))
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}

	nodePodAssignments, err := s.engine.VirtualClusterAccess().GetNodePodAssignments(r.Context())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	webutil.Log(w, fmt.Sprintf("Congrats! Scenario-%s Successful!", s.Name()))
	webutil.LogNodePodAssignments(w, s.Name(), nodePodAssignments)
	slog.Info("Execution of scenario " + s.Name() + " completed!")

}

var _ scalesim.Scenario = (*scenarioC)(nil)

func (s scenarioC) Description() string {
	return "Scale 2 Worker Pool with machine type m5.large, m5.2xlarge with replicas of small and large pods"
}

func (s scenarioC) ShootName() string {
	return shootName
}

func (s scenarioC) Name() string {
	return scenarioName
}
