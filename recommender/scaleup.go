package recommender

import (
	"context"
	"fmt"
	"math"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

/*
	for {
		unscheduledPods = determine unscheduled pods
		if noUnscheduledPods then exit early
		- runSimulationAndDetermineWinner
 		  - Start a go-routine for each of candidate node-group which are eligible
				- eligibility: max is not yet reached for that node-group
              For each go-routine:
                Setup:
                    - create a unique that will get added to all nodes and pods
                	- copy previous winner nodes and a taint.
                	- copy the deployed pods with node names assigned and add toleration to the taint.
	            - scale up one node, add a taint and only copy of pods will have toleration to that taint.
                - copy of unscheduled pods, add a toleration for this taint.
                - wait for pods to be scheduled
                - compute node score.
	}
*/

type StrategyWeights struct {
	LeastWaste float64
	LeastCost  float64
}

type Recommendation map[string]int

type Recommender struct {
	engine          scalesim.Engine
	shootNodes      []corev1.Node
	scenarioName    string
	shootName       string
	strategyWeights StrategyWeights
	logWriter       http.ResponseWriter
}

type nodePool struct {
	name string
	zone string
	max  int
}

func NewRecommender(engine scalesim.Engine, shootNodes []corev1.Node, scenarioName, shootName string, strategyWeights StrategyWeights, logWriter http.ResponseWriter) *Recommender {
	return &Recommender{
		engine:          engine,
		shootNodes:      shootNodes,
		scenarioName:    scenarioName,
		shootName:       shootName,
		strategyWeights: strategyWeights,
		logWriter:       logWriter,
	}
}

func (r *Recommender) Run(ctx context.Context) (Recommendation, error) {
	recommendation := make(Recommendation)
	unscheduledPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		return recommendation, err
	}
	var runCounter int
	shoot, err := r.getShoot()
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return recommendation, err
	}
	for {
		runCounter++
		webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d started...", runCounter))
		if len(unscheduledPods) == 0 {
			webutil.Log(r.logWriter, "All pods are scheduled. Exiting the loop...")
			break
		}
		nodeScores, unscheduledPods := r.runSimulationAndDetermineWinner(ctx, shoot, unscheduledPods)
		if len(nodeScores) == 0 {
			webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d, no winner could be identified. This will happen when no pods could be assgined. No more runs are required, exiting early", runCounter))
			break
		}
		winningNodeScore := getWinner(nodeScores)
		webutil.Log(r.logWriter, fmt.Sprintf("For scale-up recommender run #%d, winning score is: %v", runCounter, winningNodeScore))

	}

	return recommendation, nil
}

func (r *Recommender) getShoot() (*v1beta1.Shoot, error) {
	shoot, err := r.engine.ShootAccess(r.shootName).GetShootObj()
	if err != nil {
		return nil, err
	}
	return shoot, nil
}

func (r *Recommender) runSimulationAndDetermineWinner(ctx context.Context, shoot *v1beta1.Shoot, pods []corev1.Pod) ([]scalesim.NodeRunResult, []corev1.Pod) {
	return nil, nil
}

func (r *Recommender) getEligibleNodePools(shoot *v1beta1.Shoot) []nodePool {
	return nil
}

func getWinner(ns []scalesim.NodeRunResult) scalesim.NodeRunResult {
	var winner scalesim.NodeRunResult
	minScore := math.MaxFloat64
	for _, v := range ns {
		if v.CumulativeScore < minScore {
			winner = v
			minScore = v.CumulativeScore
		}
	}
	return winner
}

func (r *Recommender) logError(err error) {
	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
}
