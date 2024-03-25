package nodescorer

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"

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

func (r *Recommender) Run(ctx context.Context, unscheduledPods []corev1.Pod) (map[string]int, error) {
	startTime := time.Now()
	defer func() {
		webutil.Logf(r.logWriter, "Execution of scenario: %s completed in %v", r.scenarioName, time.Since(startTime))
	}()
	recommendation := make(map[string]int)
	var runCounter int
	shoot, err := r.engine.ShootAccess(r.shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return recommendation, err
	}
	for {
		runCounter++
		var nodeScores scalesim.NodeRunResults
		unscheduledPodCount := len(unscheduledPods)

		if unscheduledPodCount == 0 {
			webutil.Log(r.logWriter, "All pods are scheduled. Exiting the loop...")
			break
		}
		webutil.Log(r.logWriter, fmt.Sprintf("Scenario-Run #%d", runCounter))

		nodeScores = r.computeNodeScores(ctx, shoot, unscheduledPods)
		if len(nodeScores) == 0 {
			webutil.Log(r.logWriter, fmt.Sprintf("In Scenario-Run #%d, no pods could be assgined, exiting early", runCounter))
			break
		}
		winnerNodeScore := nodeScores.GetWinner()
		webutil.Log(r.logWriter, "Winning Score: "+winnerNodeScore.String())
		recommendation[winnerNodeScore.Pool.Name]++

		checkEventsSince := time.Now()
		scaledNode, err := r.scaleWorker(ctx, winnerNodeScore.Pool)
		if err != nil {
			r.logError(err)
			return recommendation, err
		}
		if err = r.engine.VirtualClusterAccess().CreatePods(ctx, unscheduledPods...); err != nil {
			r.logError(err)
			return recommendation, err
		}

		_, _, err = simutil.WaitForAndRecordPodSchedulingEvents(ctx, r.engine.VirtualClusterAccess(), r.logWriter, checkEventsSince, unscheduledPods, 10*time.Second)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return recommendation, err
		}

		assignedPods, err := simutil.GetPodsAssignedToNode(ctx, r.engine.VirtualClusterAccess(), scaledNode.Name)
		if err != nil {
			r.logError(err)
			return recommendation, err
		}
		webutil.Log(r.logWriter, fmt.Sprintf("At the end of run #%d, winner node %s is assigned pods: %v", runCounter, scaledNode.Name, simutil.PodNames(assignedPods)))
		if len(assignedPods) == 0 {
			webutil.Log(r.logWriter, "No pods are assigned to the winning scaled node "+scaledNode.Name)
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error")
			return recommendation, err
		}
		unscheduledPods = simutil.DeleteAssignedPods(unscheduledPods, assignedPods)
		webutil.Log(r.logWriter, fmt.Sprintf("Removing unscheduled pods #%d, at the end of run #%d", len(unscheduledPods), runCounter))
		if err = r.engine.VirtualClusterAccess().DeletePods(ctx, unscheduledPods...); err != nil {
			r.logError(err)
			return recommendation, err
		}
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

func (r *Recommender) computeNodeScores(ctx context.Context, shoot *v1beta1.Shoot, candidatePods []corev1.Pod) scalesim.NodeRunResults {
	nodeScores := scalesim.NodeRunResults(make(map[string]scalesim.NodeRunResult))
	var checkEventsSince time.Time
	for _, pool := range shoot.Spec.Provider.Workers {
		//if err = r.engine.VirtualClusterAccess().DeletePods(ctx, candidatePods...); err != nil {
		//	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
		//	return nil, candidatePods
		//}
		webutil.Logf(r.logWriter, "Scaling workerpool %s...", pool.Name)
		checkEventsSince = time.Now()
		scaledNode, err := simutil.CreateNodeInWorkerGroup(ctx, r.engine.VirtualClusterAccess(), &pool)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil
		}
		if scaledNode == nil {
			webutil.Log(r.logWriter, "No new node can be created for pool "+pool.Name+" as it has reached its max. Skipping this pool.")
			continue
		}
		if err = r.engine.VirtualClusterAccess().CreatePods(ctx, candidatePods...); err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil
		}
		_, _, err = simutil.WaitForAndRecordPodSchedulingEvents(ctx, r.engine.VirtualClusterAccess(), r.logWriter, checkEventsSince, candidatePods, 10*time.Second)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil
		}
		allPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil
		}
		candidatePods = simutil.GetMatchingPods(allPods, candidatePods)
		nodeScore, err := computeNodeRunResult(r.strategyWeights, scaledNode, candidatePods, shoot.Spec.Provider.Workers)
		if err != nil {
			webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
			return nil
		}
		webutil.Log(r.logWriter, "Resetting virtual cluster state for next scale-up...")
		if err = r.resetRunState(ctx, scaledNode.Name, candidatePods); err != nil {
			return nil
		}
		if nodeScore.NumAssignedPodsToNode == 0 {
			webutil.Log(r.logWriter, "No pods are scheduled on the candidate scaled node "+scaledNode.Name+". This node group will not be considered in this run")
			continue
		}
		nodeScores[scaledNode.Name] = nodeScore
		webutil.Log(r.logWriter, fmt.Sprintf("Node score for %s: %v", scaledNode.Name, nodeScores[scaledNode.Name]))
	}
	return nodeScores
}

func (r *Recommender) logError(err error) {
	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
}

func (r *Recommender) resetRunState(ctx context.Context, nodeName string, pods []corev1.Pod) error {
	webutil.Log(r.logWriter, "Deleting scaled node..."+nodeName)
	if err := r.engine.VirtualClusterAccess().DeleteNode(ctx, nodeName); err != nil {
		webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
		return err
	}
	if err := r.engine.VirtualClusterAccess().DeletePods(ctx, pods...); err != nil {
		webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
		return err
	}
	return nil
}
