package nodescorer

import (
	"context"
	"fmt"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
	corev1 "k8s.io/api/core/v1"
)

type Strategy string

const (
	LeastCostStrategy  Strategy = "LeastCost"
	LeastWasteStrategy Strategy = "LeastWaste"
)

type Recommender struct {
	engine       scalesim.Engine
	scenarioName string
	shootName    string
	strategy     Strategy
	logWriter    http.ResponseWriter
	outCh        chan string
}

func NewRecommender(engine scalesim.Engine, scenarioName, shootName string, strategy Strategy, logWriter http.ResponseWriter) *Recommender {
	return &Recommender{
		engine:       engine,
		scenarioName: scenarioName,
		shootName:    shootName,
		strategy:     strategy,
		logWriter:    logWriter,
		outCh:        make(chan string, 2),
	}
}

func (r *Recommender) Run(ctx context.Context) {
	unscheduledPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		r.logError(err)
		return
	}
	var runCounter int
	for {
		runCounter++
		unscheduledPodCount, err := simutil.GetUnscheduledPodCount(ctx, r.engine.VirtualClusterAccess())
		if err != nil {
			r.logError(err)
			return
		}
		if unscheduledPodCount == 0 {
			webutil.Log(r.logWriter, "All pods are scheduled. Exiting the loop...")
			break
		}
		webutil.Log(r.logWriter, fmt.Sprintf("Scenario-Run #%d", runCounter))

		nodeScores := r.computeNodeScores(unscheduledPods)
		r.computeAndPublishWinningScore(nodeScores)
	}
}

func (r *Recommender) computeAndPublishWinningScore(scores []scalesim.NodeRunResult) {

}

func (r *Recommender) computeNodeScores(candidatePods []corev1.Pod) []scalesim.NodeRunResult {
	return nil
}

func (r *Recommender) logError(err error) {
	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
}
