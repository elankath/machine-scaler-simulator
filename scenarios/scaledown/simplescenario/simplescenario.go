package simplescenario

import (
	"context"
	"github.com/elankath/scaler-simulator/scenarios/scaledown"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
)

var (
	shootName    = "scenario-a"
	scenarioName = "simple"
)

const (
	largePodPath = "scenarios/scaledown/assets/podLarge.yaml"
	smallPodPath = "scenarios/scaledown/assets/podSmall.yaml"
)

type simpleScaleDown struct {
	engine scalesim.Engine
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &simpleScaleDown{
		engine: engine,
	}
}
func (s *simpleScaleDown) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	webutil.Log(w, "Commencing scenario: "+s.Name()+"...")
	webutil.Log(w, "Clearing virtual cluster..")

	smallCount := webutil.GetIntQueryParam(r, "small", 10)
	largeCount := webutil.GetIntQueryParam(r, "large", 1)

	podRequests := map[string]int{
		smallPodPath: smallCount,
		largePodPath: largeCount,
	}
	scaledown.NewScenarioRunner(s.engine, shootName, scenarioName, podRequests).Run(r.Context(), w, s.setUp)
}

var _ scalesim.Scenario = (*simpleScaleDown)(nil)

func (s *simpleScaleDown) Description() string {
	return "Scale 2 Worker Pool with machine type m5.large, m5.2xlarge with replicas of small and large pods"
}

func (s *simpleScaleDown) ShootName() string {
	return shootName
}

func (s *simpleScaleDown) Name() string {
	return scenarioName
}

// TODO: implement me
func (s *simpleScaleDown) setUp(ctx context.Context) error {
	return nil
}
