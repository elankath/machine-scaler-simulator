package score4

import (
	"fmt"
	"net/http"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/nodescorer"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/virtualcluster"
	"github.com/elankath/scaler-simulator/webutil"
	corev1 "k8s.io/api/core/v1"
)

var shootName string
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
	shootName = webutil.GetStringQueryParam(r, "shoot", "")
	if shootName == "" {
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
	leastWasteWeight, err := webutil.GetFloatQueryParam(r, "leastWaste", 1.0)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	leastCostWeight, err := webutil.GetFloatQueryParam(r, "leastCost", 1.0)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	podOrder := webutil.GetStringQueryParam(r, "podOrder", "noorder")
	withTSC := webutil.GetStringQueryParam(r, "withTSC", "false")

	allPods := make([]corev1.Pod, 0, smallCount+largeCount)
	smallPodLabels := map[string]string{
		"app.kubernetes.io/name": "score4",
		"foo":                    "bar",
	}
	largePodLabels := map[string]string{
		"app.kubernetes.io/name": "score4",
		"foo":                    "bar2",
	}

	if withTSC == "true" {
		webutil.Log(w, "Creating Pods with TopologySpreadConstraints")
		topologyKeyForZone := "failure-domain.beta.kubernetes.io/zone"
		maxSkew := 1
		matchingTSCLabels := map[string]string{
			"foo": "bar",
		}
		smallPods, err := constructPodsWithTSC("small", virtualcluster.BinPackingSchedulerName, "", "5Gi", "100m", smallCount, &topologyKeyForZone, &maxSkew, matchingTSCLabels, smallPodLabels)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		topologyKeyForHostname := "kubernetes.io/hostname"
		matchingTSCLabels = map[string]string{
			"foo": "bar2",
		}
		largePods, err := constructPodsWithTSC("large", virtualcluster.BinPackingSchedulerName, "", "12Gi", "200m", largeCount, &topologyKeyForHostname, &maxSkew, matchingTSCLabels, largePodLabels)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		allPods = append(allPods, smallPods...)
		allPods = append(allPods, largePods...)
	} else {
		delete(smallPodLabels, "foo")
		smallPods, err := constructPodsWithoutTSC("small", virtualcluster.BinPackingSchedulerName, "", "5Gi", "100m", smallCount, smallPodLabels)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}

		delete(largePodLabels, "foo")
		largePods, err := constructPodsWithoutTSC("large", virtualcluster.BinPackingSchedulerName, "", "12Gi", "200m", largeCount, largePodLabels)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		allPods = append(allPods, smallPods...)
		allPods = append(allPods, largePods...)
	}

	recommender := nodescorer.NewRecommender(s.engine, scenarioName, shootName, podOrder, scalesim.StrategyWeights{
		LeastWaste: leastWasteWeight,
		LeastCost:  leastCostWeight,
	}, w)

	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	startTime := time.Now()
	recommendation, err := recommender.Run(r.Context(), shoot, allPods)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		return

	}
	webutil.Log(w, fmt.Sprintf("Execution of scenario: %s completed in %f seconds", scenarioName, time.Since(startTime).Seconds()))
	webutil.Log(w, fmt.Sprintf("Recommendation: %s", recommendation.String()))
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

func constructPodsWithTSC(namePrefix string, schedulerName string, nodeName string, memRequest, cpuRequest string, count int, topologyKey *string, maxSkew *int, matchingTSCLabels map[string]string, labels map[string]string) ([]corev1.Pod, error) {
	return constructPods(namePrefix, schedulerName, nodeName, memRequest, cpuRequest, count, topologyKey, maxSkew, matchingTSCLabels, labels)
}

func constructPodsWithoutTSC(namePrefix string, schedulerName string, nodeName string, memRequest, cpuRequest string, count int, labels map[string]string) ([]corev1.Pod, error) {
	return constructPods(namePrefix, schedulerName, nodeName, memRequest, cpuRequest, count, nil, nil, nil, labels)
}

func constructPods(namePrefix string, schedulerName string, nodeName string, memRequest, cpuRequest string, count int, topologyKey *string, maxSkew *int, matchingTSCLabels map[string]string, labels map[string]string) ([]corev1.Pod, error) {
	pods := make([]corev1.Pod, 0, count)
	for i := 0; i < count; i++ {
		suffix, err := simutil.GenerateRandomString(4)
		if err != nil {
			return nil, err
		}
		p, err := simutil.NewPodBuilder().
			Name(namePrefix+"-"+suffix).
			SchedulerName(schedulerName).
			AddLabels(labels).
			NodeName(nodeName).
			RequestMemory(memRequest).
			RequestCPU(cpuRequest).
			TopologySpreadConstraint(topologyKey, maxSkew, matchingTSCLabels).
			Build()
		if err != nil {
			return nil, err
		}
		pods = append(pods, *p)
	}
	return pods, nil
}
