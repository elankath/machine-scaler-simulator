package recommender

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/webutil"
	corev1 "k8s.io/api/core/v1"
)

// ScaleDownOrderedByDescendingCost scales down the nodes in the cluster ordered by descending cost. It does the following
//  0. Order all existing nodes by their cost.
//  1. Iterate over all existing ordered nodes, for each node:
//     1.1 Taint the node with NoSchedule.
//     1.2 Get the pods on the node and deploy a copy of the pods with new names in the cluster.
//     1.3 Check whether there are un-scheduled pods.
//     If len(unscheduledPods) == 0 {
//     then delete the node from virtual cluster and record as recommendation
//     else {
//     mark as essential (not to be removed).
//     Delete the newly deployed pods.
//     Un-taint the node if this node is essential.
//     }
func ScaleDownOrderedByDescendingCost(ctx context.Context, vca scalesim.VirtualClusterAccess, w http.ResponseWriter, nodes []corev1.Node) ([]string, error) {
	startTime := time.Now()
	defer func() {
		executionDuration := time.Since(startTime)
		webutil.Log(w, fmt.Sprintf("ScaleDownOrderedByDescendingCost scale down recommender took %f seconds", executionDuration.Seconds()))
	}()
	slices.SortFunc(nodes, simutil.ComparePriceDescending)
	var deletableNodeNames []string

	for _, n := range nodes {
		if simutil.IsExistingNode(&n) {
			continue
		}
		assignedPods, err := simutil.GetPodsOnNode(ctx, vca, n.Name)
		if err != nil {
			return deletableNodeNames, err
		}

		webutil.Log(w, "Deleting candidate node and corresponding pods: "+n.Name)
		if err = simutil.DeleteNodeAndPods(ctx, w, vca, &n, assignedPods); err != nil {
			return deletableNodeNames, err
		}

		if len(assignedPods) == 0 {
			deletableNodeNames = append(deletableNodeNames, n.Name)
			webutil.Log(w, fmt.Sprintf("Node %s has no pods. Adding it to deletion candidates", n.Name))
			continue
		}

		adjustedPods := simutil.AdjustPods(assignedPods)
		adjustedPodNames := simutil.PodNames(adjustedPods)
		webutil.Log(w, fmt.Sprintf("Deploying adjusted Pods...: %s", adjustedPodNames))
		deployStartTime := time.Now()
		if err = vca.CreatePods(ctx, "", adjustedPods...); err != nil {
			return deletableNodeNames, err
		}
		scheduledPodNames, unscheduledPodNames, err := simutil.WaitForAndRecordPodSchedulingEvents(ctx, vca, w, deployStartTime, adjustedPods, 10*time.Second)
		if err != nil {
			return deletableNodeNames, err
		} else {
			webutil.Log(w, fmt.Sprintf("Scheduled pods: %v, unscheduled pods: %v", scheduledPodNames, unscheduledPodNames))
			if len(unscheduledPodNames) != 0 {
				webutil.Log(w, fmt.Sprintf("Node %s CANNOT be removed since it will result in %d unscheduled pods", n.Name, len(unscheduledPodNames)))
				if err = vca.DeletePods(ctx, adjustedPods...); err != nil {
					return deletableNodeNames, err
				}
				webutil.Log(w, fmt.Sprintf("Recreating node %s and corresponding pods %s", n.Name, simutil.PodNames(assignedPods)))
				if err = recreateNodeWithPods(ctx, vca, &n, assignedPods); err != nil {
					return deletableNodeNames, err
				}
			} else {
				webutil.Log(w, fmt.Sprintf("Node %s can be removed, adding it to deletion candidates", n.Name))
				deletableNodeNames = append(deletableNodeNames, n.Name)
			}
		}
	}
	return deletableNodeNames, nil
}

func recreateNodeWithPods(ctx context.Context, vca scalesim.VirtualClusterAccess, node *corev1.Node, pods []corev1.Pod) error {
	if err := vca.AddNodesAndUpdateLabels(ctx, node); err != nil {
		return err
	}
	if err := vca.RemoveAllTaintsFromVirtualNode(ctx, node.Name); err != nil {
		return err
	}
	for _, pod := range pods {
		schedName := pod.Spec.SchedulerName
		if err := vca.CreatePodsWithNodeAndScheduler(ctx, schedName, node.Name, pod); err != nil {
			return err
		}
	}
	return nil
}
