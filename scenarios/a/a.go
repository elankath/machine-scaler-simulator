package a

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

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
	webutil.Log(w, "Clearing virtual cluster..")
	err := s.engine.VirtualClusterAccess().ClearAll(r.Context())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	err = s.engine.SyncVirtualNodesWithShoot(r.Context(), shootName)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	scaleStartTime := time.Now()
	podCount := webutil.GetIntQueryParam(r, "podCount", 4)
	podSpecPath := "scenarios/a/pod.yaml"
	webutil.Log(w, fmt.Sprintf("Applying %d replicas of pod spec: %s and waiting for: %d secs", podCount, podSpecPath))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, podCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		return
	}
	_, err = s.engine.ScaleWorkerPoolsTillMaxOrNoUnscheduledPods(r.Context(), s.Name(), scaleStartTime, shoot, w)
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
