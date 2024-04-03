package scaledown

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"log/slog"
	"net/http"
	"slices"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/recommender"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
)

type ScenarioRunner struct {
	engine       scalesim.Engine
	shootName    string
	scenarioName string
	podRequests  map[string]int
}

type SetupScenarioFunc func(ctx context.Context) error

func NewScenarioRunner(engine scalesim.Engine, shootName, scenarioName string, podRequests map[string]int) *ScenarioRunner {
	return &ScenarioRunner{
		engine:       engine,
		shootName:    shootName,
		scenarioName: scenarioName,
		podRequests:  podRequests,
	}
}

func (s ScenarioRunner) Run(ctx context.Context, w http.ResponseWriter) {
	if err := s.resetVirtualCluster(ctx, s.engine, s.shootName); err != nil {
		webutil.InternalError(w, err)
		return
	}
	shoot, err := s.engine.ShootAccess(s.shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	if err = s.createNodesInVirtualCluster(ctx, w, shoot); err != nil {
		webutil.Log(w, "Execution of scenario: "+s.scenarioName+" completed with error: "+err.Error())
		webutil.InternalError(w, err)
		slog.Error("Execution of scenario: "+s.scenarioName+" ran into error", "error", err)
		return
	}

	//if err := setupFunc(ctx); err != nil {
	//	webutil.Log(w, "Execution of scenario: "+s.scenarioName+" completed with error: "+err.Error())
	//	slog.Error("Execution of scenario: "+s.scenarioName+" ran into error", "error", err)
	//	webutil.InternalError(w, err)
	//	return
	//}

	if err := s.deployPods(ctx, w, s.podRequests); err != nil {
		webutil.Log(w, "Execution of scenario: "+s.scenarioName+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.scenarioName+" ran into error", "error", err)
		webutil.InternalError(w, err)
		return
	}
	originalNodePodAssignments := s.printAndGetNodePodAssignments(ctx, w)

	nodes, err := s.engine.VirtualClusterAccess().ListNodes(ctx)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	scaleDownRecommendation, err := recommender.ScaleDownOrderedByDescendingCost(ctx, s.engine.VirtualClusterAccess(), w, nodes)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.scenarioName+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.scenarioName+" ran into error", "error", err)
		webutil.InternalError(w, err)
	}
	originalNodeNames := lo.Map(originalNodePodAssignments, func(npa scalesim.NodePodAssignment, _ int) string {
		return npa.NodeName
	})
	webutil.Log(w, fmt.Sprintf("Initial Nodes in the cluster: %s", originalNodeNames))
	scaleDownRecommendation = lo.Filter(scaleDownRecommendation, func(nodeName string, _ int) bool {
		return slices.Contains(originalNodeNames, nodeName)
	})
	webutil.Log(w, fmt.Sprintf("Recommendation for Scale-Down: %s", scaleDownRecommendation))
	webutil.Log(w, "Scenario-End: "+s.scenarioName)
}

func (s ScenarioRunner) printAndGetNodePodAssignments(ctx context.Context, w http.ResponseWriter) []scalesim.NodePodAssignment {
	nodePodAssignments, err := simutil.GetNodePodAssignments(ctx, s.engine.VirtualClusterAccess())
	npaAsString, err := simutil.AsJson(nodePodAssignments)
	webutil.Log(w, fmt.Sprintf("NodePodAssignments BEFORE Scale-Down are: %s", npaAsString))
	if err != nil {
		webutil.InternalError(w, err)
		return nil
	}
	return nodePodAssignments
}

func (s ScenarioRunner) deployPods(ctx context.Context, w http.ResponseWriter, podRequests map[string]int) error {
	deployStartTime := time.Now()
	for podYamlPath, numPods := range podRequests {
		webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podYamlPath, numPods))
		if err := s.engine.VirtualClusterAccess().CreatePodsFromYaml(ctx, podYamlPath, numPods); err != nil {
			return err
		}
	}
	timeout := 30 * time.Second
	webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeout.Seconds())
	_, err := simutil.WaitTillNoUnscheduledPodsOrTimeout(ctx, s.engine.VirtualClusterAccess(), timeout, deployStartTime)
	if err != nil {
		return err
	}
	return nil
}

func (s ScenarioRunner) resetVirtualCluster(ctx context.Context, engine scalesim.Engine, shootName string) error {
	if err := engine.VirtualClusterAccess().ClearAll(ctx); err != nil {
		return err
	}
	if err := engine.SyncVirtualNodesWithShoot(ctx, s.shootName); err != nil {
		return err
	}
	return nil
}

func (s ScenarioRunner) createNodesInVirtualCluster(ctx context.Context, w http.ResponseWriter, shoot *v1beta1.Shoot) error {
	webutil.Log(w, "Scenario-Start: Scaling worker pools in virtual cluster till worker pool max...")
	numCreatedNodes, err := s.engine.ScaleAllWorkerPoolsTillMax(ctx, s.scenarioName, shoot, w)
	if err != nil {
		return err
	}
	webutil.Log(w, fmt.Sprintf("Created %d total virtual nodes", numCreatedNodes))
	return nil
}
