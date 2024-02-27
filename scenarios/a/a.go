package a

import (
	"fmt"
	"github.com/elankath/scaler-simulator/scaleutil"
	"log/slog"
	"net/http"
	"os"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
)

var shootName = "scenario-a"
var scenarioName = "A"

type scenarioA struct {
	engine scalesim.Engine
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioA{
		engine: engine,
	}
}

func (s *scenarioA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	webutil.Log(w, "Commencing scenario: "+s.Name()+"...")
	//webutil.Log(w, "Tainting existing nodes in shoot: "+shootName+"...")

	webutil.Log(w, "Creating pods in shoot: "+shootName+"...")
	podCount := webutil.GetIntQueryParam(r, "replicas", 4)
	podSpecPath := "scenarios/a/pod.yaml"
	workingDir, _ := os.Getwd()
	absolutePath := workingDir + "/" + podSpecPath
	webutil.Log(w, fmt.Sprintf("Applying %d replicas of pod spec: %s...", podCount, podSpecPath))
	err := s.engine.ShootAccess(shootName).CreatePods(absolutePath, podCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	webutil.Log(w, "Clearing virtual cluster..")
	err = s.engine.VirtualClusterAccess().ClearAll(r.Context())
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
	webutil.Log(w, fmt.Sprintf("Getting shoot object for shoot: %s using kubectl ...", shootName))
	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	scaleStartTime := time.Now()
	webutil.Log(w, "Scenario-Start: Scaling worker pools in virtual cluster till worker pool max...")
	numCreatedNodes, err := s.engine.ScaleAllWorkerPoolsTillMax(r.Context(), s.Name(), shoot, w)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Created %d total virtual nodes", numCreatedNodes))

	unscheduledPods, err := s.engine.ShootAccess(shootName).GetUnscheduledPods()
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}

	if len(unscheduledPods) != 0 {
		err = s.engine.VirtualClusterAccess().CreatePods(r.Context(), unscheduledPods...)
		if err != nil {
			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
			slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
			return
		}
	}

	timeoutSecs := 30 * time.Second
	webutil.Logf(w, "Waiting till there are no unschedulable pods in virtual cluster or timeout of %.2f secs", timeoutSecs.Seconds())
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

	err = simutil.PrintScheduledPodEvents(r.Context(), s.engine.VirtualClusterAccess(), scaleStartTime, w)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}

	err = scaleutil.ParseRecommendationsAndScaleUp(s.engine.ShootAccess(shootName), recommendation, w)
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

var _ scalesim.Scenario = (*scenarioA)(nil)

func (s scenarioA) Description() string {
	return "Scale Single Worker Pool with machine type m5.large with Pod(s) of 5Gb"
}

func (s scenarioA) ShootName() string {
	return shootName
}

func (s scenarioA) Name() string {
	return scenarioName
}
