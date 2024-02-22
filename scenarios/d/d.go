package d

import (
	"fmt"
	"github.com/elankath/scaler-simulator/simutil"
	"log/slog"
	"net/http"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
)

var shootName = "scenario-d1"
var scenarioName = "D"

type scenarioD struct {
	engine scalesim.Engine
}

func (s *scenarioD) Description() string {
	return "Scenario D tests scaleup of worker pools when there are unschedulable pods with topology spread constraints for different zones."
}

func (s *scenarioD) Name() string {
	return scenarioName
}

func (s *scenarioD) ShootName() string {
	return shootName
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioD{
		engine: engine,
	}
}

// Scenario D tests scaleup of worker pools when there are unschedulable pods with topology spread constraints for different zones.
func (s *scenarioD) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	scaleStartTime := time.Now()
	webutil.Log(w, "Scaling till worker pool x zone max...")
	numCreatedNodes, err := s.engine.ScaleWorkerPoolsTillNumZonesMultPoolsMax(r.Context(), s.Name(), shoot, w)
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

	podSpecPath := "scenarios/d/podA.yaml"
	podCount := webutil.GetIntQueryParam(r, "podA", 5)
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

	webutil.Log(w, fmt.Sprintf("Deployed %d Pods..wait for scheduler to sync...", podCount))

	timeoutSecs := 30 * time.Second
	webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeoutSecs.Seconds())
	err = simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), timeoutSecs, scaleStartTime)
	if err != nil { // TODO: too much repetition move this to scenarios as utility function
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
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

	webutil.Log(w, fmt.Sprintf("Congrats! Scenario-%s Successful!", s.Name()))
	webutil.LogNodePodAssignments(w, s.Name(), nodePodAssignments)
	slog.Info("Execution of scenario " + s.Name() + " completed!")
	webutil.Log(w, "Recommendation for Scaleup: "+recommendation.String())

}
