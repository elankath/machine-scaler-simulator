package a

import (
	"fmt"
	"log/slog"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
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
	err := s.engine.SyncNodes(r.Context(), shootName)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	if err := s.engine.VirtualClusterAccess().RemoveTaintFromNode(r.Context()); err != nil {
		webutil.InternalError(w, err)
		return
	}
	podCount := webutil.GetIntQueryParam(r, "podCount", 3)
	podSpecPath := "scenarios/a/pod.yaml"
	waitSecs := 15
	webutil.Log(w, fmt.Sprintf("Applying %d replicas of pod spec: %s and waiting for: %d secs", podCount, podSpecPath, waitSecs))
	err = s.engine.ApplyPod(r.Context(), podSpecPath, podCount, waitSecs)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		return
	}
	_, err = s.engine.ScaleWorkerPoolsTillMaxOrNoUnscheduledPods(r.Context(), s.Name(), shoot, w)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
	} else {
		webutil.Log(w, "CONGRATS! Execution of scenario: "+s.Name()+"was successful!")
		slog.Info("Execution of scenario A completed!")
	}
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
