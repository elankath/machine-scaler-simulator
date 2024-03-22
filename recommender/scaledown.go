package recommender

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/simutil"
	"github.com/elankath/scaler-simulator/virtualcluster"
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
		webutil.Log(w, "Tainting node: "+n.Name)
		if err = vca.AddTaintToNode(ctx, &n); err != nil {
			return deletableNodeNames, err
		}
		if len(assignedPods) == 0 {
			deletableNodeNames = append(deletableNodeNames, n.Name)
			continue
		}

		adjustedPods := simutil.AdjustPods(assignedPods)
		adjustedPodNames := simutil.PodNames(adjustedPods)
		webutil.Log(w, fmt.Sprintf("Deploying adjusted Pods...: %s", adjustedPodNames))
		if err = vca.CreatePods(ctx, virtualcluster.BinPackingSchedulerName, adjustedPods...); err != nil {
			return deletableNodeNames, err
		}

		var essentialNode bool
		numUnscheduled, err := simutil.WaitAndGetUnscheduledPodCount(ctx, vca, 10)
		webutil.Log(w, fmt.Sprintf("candidate node: %s, numUnscheduledPods: %d, after deployment of adjusted pods: %s, error (if any):  %s", n.Name, numUnscheduled, adjustedPodNames, err))
		if err != nil {
			return deletableNodeNames, err
		} else {
			if numUnscheduled == 0 {
				if err = simutil.DeleteNodeAndPods(ctx, w, vca, &n, assignedPods); err != nil {
					return deletableNodeNames, err
				}
			} else {
				webutil.Log(w, fmt.Sprintf("Node %s CANNOT be removed since it will result in %d unscheduled pods", n.Name, numUnscheduled))
				essentialNode = true
			}
		}

		if essentialNode {
			if err = vca.DeletePods(ctx, adjustedPods...); err != nil {
				return deletableNodeNames, err
			}
			if err = vca.RemoveTaintFromVirtualNode(ctx, n.Name); err != nil {
				return deletableNodeNames, err
			}
			webutil.Log(w, fmt.Sprintf("Removed Taint from node %s. Deleted adjusted pods: %s", n.Name, adjustedPodNames))
		} else {
			deletableNodeNames = append(deletableNodeNames, n.Name)
		}
	}
	return deletableNodeNames, nil
}
