package score4

import (
	"fmt"
	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/nodescorer"
	"github.com/elankath/scaler-simulator/webutil"
	"net/http"
)

var shootName = "scenario-c2"
var scenarioName = "score4"

type scenarioscore4 struct {
	engine scalesim.Engine
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioscore4{
		engine: engine,
	}
}

// Scenario C will first scale up nodes in all worker pools of scenario-c shoot to MAX
// Then deploy Pods small and large according to count.
// Then wait till all Pods are scheduled or till timeout.
func (s *scenarioscore4) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	smallCount := webutil.GetIntQueryParam(r, "small", 10)
	largeCount := webutil.GetIntQueryParam(r, "large", 2)

	podSpecPath := "scenarios/score4/podLarge.yaml"
	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, largeCount))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, largeCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	podSpecPath = "scenarios/score4/podSmall.yaml"
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

	recommender := nodescorer.NewRecommender(s.engine, scenarioName, shootName, nodescorer.StrategyWeights{
		LeastWaste: 1.0,
		LeastCost:  1.5,
	}, w)

	recommendation, err := recommender.Run(r.Context())
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		return
	}
	webutil.Log(w, fmt.Sprintf("Recommendation: %+v", recommendation))
	webutil.Log(w, fmt.Sprintf("Scenario-%s Completed!", s.Name()))
}

var _ scalesim.Scenario = (*scenarioscore4)(nil)

func (s scenarioscore4) Description() string {
	return "Scale 2 Worker Pool with machine type m5.large, m5.2xlarge with replicas of small and large pods"
}

func (s scenarioscore4) ShootName() string {
	return shootName
}

func (s scenarioscore4) Name() string {
	return scenarioName
}

