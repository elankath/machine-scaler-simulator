package tscscenario

import (
	"github.com/elankath/scaler-simulator/scenarios/scaledown"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
)

var (
	shootName    = "scenario-c1"
	scenarioName = "tsc"
)

const (
	largePodPath = "scenarios/scaledown/assets/podLargeWithTSC.yaml"
	smallPodPath = "scenarios/scaledown/assets/podSmallWithTSC.yaml"
)

type tscScaleDown struct {
	engine scalesim.Engine
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &tscScaleDown{
		engine: engine,
	}
}
func (s *tscScaleDown) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	webutil.Log(w, "Commencing scenario: "+s.Name()+"...")
	webutil.Log(w, "Clearing virtual cluster..")

	smallCount := webutil.GetIntQueryParam(r, "small", 10)
	largeCount := webutil.GetIntQueryParam(r, "large", 1)

	podRequests := map[string]int{
		smallPodPath: smallCount,
		largePodPath: largeCount,
	}
	scaledown.NewScenarioRunner(s.engine, shootName, scenarioName, podRequests).Run(r.Context(), w)
}

var _ scalesim.Scenario = (*tscScaleDown)(nil)

func (s *tscScaleDown) Description() string {
	return "Scale 2 Worker Pool with machine type m5.large, m5.2xlarge with replicas of small and large pods"
}

func (s *tscScaleDown) ShootName() string {
	return shootName
}

func (s *tscScaleDown) Name() string {
	return scenarioName
}
