package p

import (
	"fmt"
	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
	"log/slog"
	"net/http"
	"time"
)

var shootName = "scenario-p"
var scenarioName = "P"

type scenarioP struct {
	engine scalesim.Engine
}

func (s *scenarioP) Description() string {
	return "Scenario P tests scaleup of worker pools based on declaration based priority."
}

func (s *scenarioP) Name() string {
	return scenarioName
}

func (s *scenarioP) ShootName() string {
	return shootName
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioP{
		engine: engine,
	}
}

// Scenario P tests scaling of worker pools based on priority. The assumption is the pool declared first in the shoot has the higher priority
func (s *scenarioP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	podSpecPath := "scenarios/p/podA.yaml"
	podCount := webutil.GetIntQueryParam(r, "podA", 2)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, podCount))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, podCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	webutil.Log(w, fmt.Sprintf("Deployed %d PodAs..wait for scheduler to sync...", podCount))

	podSpecPath = "scenarios/p/podB.yaml"
	podCount = webutil.GetIntQueryParam(r, "podB", 4)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, podCount))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, podCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	webutil.Log(w, fmt.Sprintf("Deployed %d PodBs..wait for scheduler to sync...", podCount))

	scaleStartTime := time.Now()

	for _, pool := range shoot.Spec.Provider.Workers {
		numCreatedNodes, err := s.engine.ScaleWorkerPoolTillMax(r.Context(), s.Name(), &pool, w)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		webutil.Log(w, fmt.Sprintf("Created %d total nodes", numCreatedNodes))
		count, err := simutil.WaitAndGetUnscheduledPodCount(r.Context(), s.engine.VirtualClusterAccess(), 8)
		if err != nil {
			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
			slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		}
		if count == 0 {
			webutil.Log(w, "All pods Scheduled!!")
			break
		}
		webutil.Log(w, fmt.Sprintf("Unscheduled pod count: %d after scaling pool %s, continuing with scale up of next worker pool", count, pool.Name))
	}

	timeoutSecs := 3 * time.Second
	webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeoutSecs.Seconds())
	_, err = simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), timeoutSecs, scaleStartTime)
	if err != nil { // TODO: too much repetition move this to scenarios as utility function
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
	}

	webutil.Log(w, "Trimming virtual cluster...")
	err = s.engine.VirtualClusterAccess().TrimCluster(r.Context())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	nodePodAssignments, err := simutil.GetNodePodAssignments(r.Context(), s.engine.VirtualClusterAccess())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	recommendation, err := simutil.GetScalerRecommendation(r.Context(), s.engine.VirtualClusterAccess(), nodePodAssignments)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}

	err = simutil.PrintScheduledPodEvents(r.Context(), s.engine.VirtualClusterAccess(), scaleStartTime, w)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}

	webutil.Log(w, fmt.Sprintf("Scenario-%s completed", s.Name()))
	webutil.LogNodePodAssignments(w, s.Name(), nodePodAssignments)
	slog.Info("Execution of scenario " + s.Name() + " completed!")
	webutil.Log(w, "Recommendation for Scaleup: "+recommendation.String())

}
