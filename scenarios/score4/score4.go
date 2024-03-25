package score4

import (
	"fmt"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/nodescorer"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/virtualcluster"
	"github.com/elankath/scaler-simulator/webutil"
	corev1 "k8s.io/api/core/v1"
)

var shootName = "scenario-c1"
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

	smallPods, err := constructPods("small", virtualcluster.BinPackingSchedulerName, "", "5Gi", "100m", smallCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	largePods, err := constructPods("large", virtualcluster.BinPackingSchedulerName, "", "12Gi", "200m", largeCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	allPods := make([]corev1.Pod, 0, smallCount+largeCount)
	allPods = append(smallPods, largePods...)

	recommender := nodescorer.NewRecommender(s.engine, scenarioName, shootName, nodescorer.StrategyWeights{
		LeastWaste: 1.0,
		LeastCost:  1.5,
	}, w)

	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	recommendation, err := recommender.Run(r.Context(), shoot, allPods)
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

func constructPods(namePrefix string, schedulerName string, nodeName string, memRequest, cpuRequest string, count int) ([]corev1.Pod, error) {
	pods := make([]corev1.Pod, 0, count)
	for i := 0; i < count; i++ {
		suffix, err := simutil.GenerateRandomString(4)
		if err != nil {
			return nil, err
		}
		p, err := simutil.NewPodBuilder().
			Name(namePrefix+"-"+suffix).
			SchedulerName(schedulerName).
			AddLabel("app.kubernetes.io/name", "score4").
			NodeName(nodeName).
			RequestMemory(memRequest).
			RequestCPU(cpuRequest).
			Build()
		if err != nil {
			return nil, err
		}
		pods = append(pods, *p)
	}
	return pods, nil
}
