package nodescorer

import (
	"errors"
	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/pricing"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"log/slog"
)

func computeNodeRunResult(strategy StrategyWeights, scaledNode *corev1.Node, podListForRun []corev1.Pod, workerPools []v1beta1.Worker) (scalesim.NodeRunResult, error) {
	var nodeScore scalesim.NodeRunResult
	nodeScore.NodeName = scaledNode.Name

	var totalAssignedPods int
	var targetNodeAssignedPods []corev1.Pod
	for _, pod := range podListForRun {
		if pod.Spec.NodeName != "" {
			totalAssignedPods++
		}
		if pod.Spec.NodeName == scaledNode.Name {
			targetNodeAssignedPods = append(targetNodeAssignedPods, pod)
		}
	}

	nodeScore.NumAssignedPodsTotal = totalAssignedPods
	nodeScore.NumAssignedPodsToNode = len(targetNodeAssignedPods)
	// TODO enhance the wastescore by considering all resources
	totalMemoryConsumed := int64(0)
	totalAllocatableMemory := scaledNode.Status.Allocatable.Memory().MilliValue()
	for _, pod := range targetNodeAssignedPods {
		for _, container := range pod.Spec.Containers {
			totalMemoryConsumed += container.Resources.Requests.Memory().MilliValue()
		}
		slog.Info("NodPodAssignment: ", "pod", pod.Name, "node", pod.Spec.NodeName, "memory", pod.Spec.Containers[0].Resources.Requests.Memory().MilliValue())
	}

	nodeScore.WasteRatio = strategy.LeastWaste * (float64(totalAllocatableMemory-totalMemoryConsumed) / float64(totalAllocatableMemory))
	nodeScore.UnscheduledRatio = float64(len(podListForRun)-totalAssignedPods) / float64(len(podListForRun))
	nodeScore.CostRatio = strategy.LeastCost * getCostRatio(scaledNode, workerPools)
	nodeScore.CumulativeScore = nodeScore.WasteRatio + nodeScore.UnscheduledRatio + nodeScore.CostRatio

	for _, pool := range workerPools {
		if scaledNode.Labels["worker.gardener.cloud/pool"] == pool.Name {
			nodeScore.Pool = &pool
			break
		}
	}

	if nodeScore.Pool == nil {
		return nodeScore, errors.New("cannot find pool for node: " + scaledNode.Name)
	}
	slog.Info("Computed node score.", "nodeScore", nodeScore)

	return nodeScore, nil
}

func getCostRatio(scaledNode *corev1.Node, workerPools []v1beta1.Worker) float64 {
	sumCost := float64(0)
	poolPrice := float64(0)
	for _, pool := range workerPools {
		price := pricing.GetPricing(pool.Machine.Type)
		sumCost += price
		if pool.Name == scaledNode.Labels["worker.gardener.cloud/pool"] {
			poolPrice = price
		}
	}
	return poolPrice / sumCost
}
