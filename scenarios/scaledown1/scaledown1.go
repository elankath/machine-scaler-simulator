package scaledown1

import (
	"fmt"
	"github.com/elankath/scaler-simulator/virtualcluster"
	"log/slog"
	"net/http"
	"slices"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
)

var shootName = "scenario-c"
var scenarioName = "scaledown1"

type scenarioScaledown1 struct {
	engine scalesim.Engine
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioScaledown1{
		engine: engine,
	}
}

// scaledown1 scenario
// NOTE: We are NOT doing this
// 1. Gets the `scale-down-utilization-threshold` from request (default: 0.5)
// 2. Scales up node groups to max and then deploys pods using the default scheduler (non bin-packing)
// 3. Removes empty nodes.
// 4. For all non-empty nodes, computes the utilization threshold.
// 5. Attempts to
// 4. Then it computes node utilization threshold (passed

// WE ARE DOING THIS.
//  1. Setup a non-optimal cluster:
//     Scales up node groups to max and then deploys pods (large/small) using the default scheduler (non bin-packing)
//     record simulationStartTime
//  2. Check nodes ordered by cost. Iterate and Run the following routine
//     2.1 Taint the node with r.Context(), NoSchedule.
//     2.2 Get the pods on the node and deploy a copy of the pods with new names in the clusteer.
//     2.3 Check whether there are un-scheduled pods. If len(uE) == 0, then delete the node from virtual cluster and record as recomendation,
//     otherwise mark as essential.
//     2.4 Delete the newly deployed pods.
//     2.5 Un-taint the node if this node is essential.

// N1(m5.xlarge/16g)- p1      //
// N2(m5.xlarge/16g)-       //
// N3(m5.4xlarge/64G)-

func (s *scenarioScaledown1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	scaleStartTime := time.Now()
	webutil.Log(w, "Scenario-Start: Scaling worker pools in virtual cluster till worker pool max...")
	numCreatedNodes, err := s.engine.ScaleAllWorkerPoolsTillMax(r.Context(), s.Name(), shoot, w)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		webutil.InternalError(w, err)
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Created %d total virtual nodes", numCreatedNodes))

	smallCount := webutil.GetIntQueryParam(r, "small", 10)
	largeCount := webutil.GetIntQueryParam(r, "large", 1)

	podSpecPath := "scenarios/c/podLarge.yaml"
	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, largeCount))
	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, largeCount)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	timeoutSecs := 5 * time.Second
	webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeoutSecs.Seconds())
	_, err = simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), timeoutSecs, scaleStartTime)
	if err != nil { // TODO: too much repetition move this to scenarios as utility function
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
	}

	podSpecPath = "scenarios/c/podSmall.yaml"
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

	timeoutSecs = 30 * time.Second
	webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeoutSecs.Seconds())
	_, err = simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), timeoutSecs, scaleStartTime)
	if err != nil { // TODO: too much repetition move this to scenarios as utility function
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
	}

	nodePodAssignments, err := simutil.GetNodePodAssignments(r.Context(), s.engine.VirtualClusterAccess())
	webutil.Log(w, fmt.Sprintf("NodePodAssignments BEFORE Scale-Down are: %s", nodePodAssignments))
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	recommendation, err := simutil.GetScalerRecommendation(r.Context(), s.engine.VirtualClusterAccess(), nodePodAssignments)
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}
	webutil.Log(w, "Recommendation for Scaleup: "+recommendation.String())

	nodes, err := s.engine.VirtualClusterAccess().ListNodes(r.Context())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	slices.SortFunc(nodes, simutil.ComparePriceDescending)

	//  2. Check nodes ordered by cost. Iterate and Run the following routine
	//     2.1 Taint the node with NoSchedule.
	//     2.2 Get the pods on the node and deploy a copy of the pods with new names in the clusteer.
	//     2.3 Check whether there are un-scheduled pods. If len(uE) == 0, then delete the node from virtual cluster and record as recomendation,
	//     otherwise mark as essential.
	//     2.4 Delete the newly deployed pods.
	//     2.5 Un-taint the node if this node is essential.

	//TODO: Move me to recommender package.
	var deletableNodeNames []string
	for _, n := range nodes {
		if simutil.IsExistingNode(&n) {
			continue
		}
		assignedPods, err := simutil.GetPodsOnNode(r.Context(), s.engine.VirtualClusterAccess(), n.Name)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		if len(assignedPods) == 0 {
			webutil.Log(w, "Empty node. Deleting "+n.Name)
			err = s.engine.VirtualClusterAccess().DeleteNode(r.Context(), n.Name)
			if err != nil {
				webutil.InternalError(w, err)
				return
			}
			deletableNodeNames = append(deletableNodeNames, n.Name)
			continue
		}
		webutil.Log(w, "Tainting node: "+n.Name)
		err = s.engine.VirtualClusterAccess().AddTaintToNode(r.Context(), &n)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		adjustedPods := simutil.AdjustPods(assignedPods)
		adjustedPodNames := simutil.PodNames(adjustedPods)
		webutil.Log(w, fmt.Sprintf("Deploying adjusted Pods...: %s", adjustedPodNames))
		err = s.engine.VirtualClusterAccess().CreatePods(r.Context(), virtualcluster.BinPackingSchedulerName, adjustedPods...)
		if err != nil {
			webutil.InternalError(w, err)
			return
		}
		//ignoring timeout err
		var essentialNode bool
		//FIXME: fix this later
		//numUnscheduled, err := simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), 10*time.Second, deployTime)
		numUnscheduled, err := simutil.WaitAndGetUnscheduledPodCount(r.Context(), s.engine.VirtualClusterAccess(), 10)
		webutil.Log(w, fmt.Sprintf("candidate node: %s, numUnscheduled: %d, after deployment of adjusted pods: %s, error (if any):  %v", n.Name, numUnscheduled, adjustedPodNames, err))
		if err != nil {
			webutil.InternalError(w, err)
		} else {
			if numUnscheduled == 0 {
				err = simutil.DeleteNodeAndPods(r.Context(), w, s.engine.VirtualClusterAccess(), &n, assignedPods)
				if err != nil {
					webutil.InternalError(w, err)
					return
				}
			} else {
				webutil.Log(w, fmt.Sprintf("Node %s CANNOT be removed since it will result in %d unscheduled pods", n.Name, numUnscheduled))
				essentialNode = true
			}
		}

		if essentialNode {
			err = s.engine.VirtualClusterAccess().DeletePods(r.Context(), adjustedPods...)
			if err != nil {
				webutil.InternalError(w, err)
				return
			}
			err = s.engine.VirtualClusterAccess().RemoveTaintFromVirtualNode(r.Context(), n.Name)
			if err != nil {
				webutil.InternalError(w, err)
				return
			}
			webutil.Log(w, fmt.Sprintf("Removed Taint from node %s. Deleted adjusted pods: %s", n.Name, adjustedPodNames))
		} else {
			deletableNodeNames = append(deletableNodeNames, n.Name)
		}
	}

	if len(deletableNodeNames) > 0 {
		webutil.Log(w, fmt.Sprintf("Scale down recommendation: %s", deletableNodeNames))
	}

	//err = simutil.PrintScheduledPodEvents(r.Context(), s.engine.VirtualClusterAccess(), scaleStartTime, w)
	//if err != nil {
	//	webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
	//	slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
	//	return
	//}

	webutil.Log(w, fmt.Sprintf("Scenario-%s Completed!", s.Name()))
	webutil.LogNodePodAssignments(w, s.Name(), nodePodAssignments)
	slog.Info("Execution of scenario " + s.Name() + " completed!")

}

var _ scalesim.Scenario = (*scenarioScaledown1)(nil)

func (s scenarioScaledown1) Description() string {
	return "Scale 2 Worker Pool with machine type m5.large, m5.2xlarge with replicas of small and large pods"
}

func (s scenarioScaledown1) ShootName() string {
	return shootName
}

func (s scenarioScaledown1) Name() string {
	return scenarioName
}
