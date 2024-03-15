package nodescorer

import (
	"context"
	"fmt"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"net/http"
	"strconv"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
	corev1 "k8s.io/api/core/v1"
)

type StrategyWeights struct {
	LeastWaste float64
	LeastCost  float64
}

type Recommender struct {
	engine          scalesim.Engine
	scenarioName    string
	shootName       string
	strategyWeights StrategyWeights
	logWriter       http.ResponseWriter
}

func NewRecommender(engine scalesim.Engine, scenarioName, shootName string, strategyWeights StrategyWeights, logWriter http.ResponseWriter) *Recommender {
	return &Recommender{
		engine:          engine,
		scenarioName:    scenarioName,
		shootName:       shootName,
		strategyWeights: strategyWeights,
		logWriter:       logWriter,
	}
}

func (r *Recommender) Run(ctx context.Context) (map[string]int, error) {
	recommendation := make(map[string]int)
	unscheduledPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		r.logError(err)
		return recommendation, err
	}
	var runCounter int
	for {
		var nodeScores scalesim.NodeRunResults
		runCounter++
		unscheduledPodCount, err := simutil.GetUnscheduledPodCount(ctx, r.engine.VirtualClusterAccess())
		if err != nil {
			r.logError(err)
			return recommendation, err
		}
		if unscheduledPodCount == 0 {
			webutil.Log(r.logWriter, "All pods are scheduled. Exiting the loop...")
			break
		}
		webutil.Log(r.logWriter, fmt.Sprintf("Scenario-Run #%d", runCounter))

		nodeScores, unscheduledPods = r.computeNodeScores(ctx, unscheduledPods)
		if len(nodeScores) == 0 {
			webutil.Log(r.logWriter, fmt.Sprintf("In Scenario-Run #%d, no pods could be assgined, exiting early", runCounter))
			break
		}
		winnerNodeScore := nodeScores.GetWinner()
		webutil.Log(r.logWriter, "Winning Score: "+winnerNodeScore.String())
		recommendation[winnerNodeScore.Pool.Name]++

		scaledNode, err := r.scaleWorker(ctx, winnerNodeScore.Pool)
		if err != nil {
			r.logError(err)
			return recommendation, err
		}
		webutil.Log(r.logWriter, "Waiting for 5 seconds for pod assignments to winning scalednode: "+scaledNode.Name)
		time.Sleep(5 * time.Second)

		assignedPods, err := simutil.GetPodsAssignedToNode(ctx, r.engine.VirtualClusterAccess(), scaledNode.Name)
		if err != nil {
			r.logError(err)
			return recommendation, err
		}
		if len(assignedPods) == 0 {
			webutil.Log(r.logWriter, "No pods are assigned to the winning scaled node "+scaledNode.Name)
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error")
			return recommendation, err
		}
		unscheduledPods = simutil.DeleteAssignedPods(unscheduledPods, assignedPods)

		webutil.Log(r.logWriter, "Num of remaining unscheduled pods: "+strconv.Itoa(len(unscheduledPods)))
	}
	return recommendation, nil
}

func (r *Recommender) scaleWorker(ctx context.Context, worker *v1beta1.Worker) (*corev1.Node, error) {
	scaledNode, err := simutil.CreateNodeInWorkerGroup(ctx, r.engine.VirtualClusterAccess(), worker)
	if err != nil {
		r.logError(err)
		return nil, err
	}
	return scaledNode, err
}

func (r *Recommender) computeNodeScores(ctx context.Context, candidatePods []corev1.Pod) (scalesim.NodeRunResults, []corev1.Pod) {
	nodeScores := scalesim.NodeRunResults(make(map[string]scalesim.NodeRunResult))
	shoot, err := r.engine.ShootAccess(r.shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return nil, candidatePods
	}

	for _, pool := range shoot.Spec.Provider.Workers {
		webutil.Logf(r.logWriter, "Scaling workerpool %s...", pool.Name)
		scaledNode, err := simutil.CreateNodeInWorkerGroup(ctx, r.engine.VirtualClusterAccess(), &pool)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil, candidatePods
		}
		webutil.Log(r.logWriter, "Waiting for 5 seconds before calculating node score...")
		time.Sleep(5 * time.Second)
		allPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil, candidatePods
		}

		candidatePods = simutil.GetMatchingPods(allPods, candidatePods)
		nodeScore, err := computeNodeRunResult(r.strategyWeights, scaledNode, candidatePods, shoot.Spec.Provider.Workers)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil, candidatePods
		}
		webutil.Log(r.logWriter, "Deleting scaled node and clearing pod assignments...")
		candidatePods, err = simutil.DeleteNodeAndResetPods(ctx, r.engine.VirtualClusterAccess(), scaledNode.Name, candidatePods)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil, candidatePods
		}
		if nodeScore.NumAssignedPodsToNode == 0 {
			webutil.Log(r.logWriter, "No pods are scheduled on the candidate scaled node "+scaledNode.Name+". This node group will not be considered in this run")
			continue
		}
		nodeScores[scaledNode.Name] = nodeScore
		webutil.Log(r.logWriter, fmt.Sprintf("Node score for %s: %v", scaledNode.Name, nodeScores[scaledNode.Name]))
	}
	return nodeScores, candidatePods
}

func (r *Recommender) logError(err error) {
	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
}
