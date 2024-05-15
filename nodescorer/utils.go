package nodescorer

import (
	"errors"
	"log/slog"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/pricing"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func computeNodeRunResult(strategy scalesim.StrategyWeights, scaledNode *corev1.Node, podListForRun []corev1.Pod, workerPools []v1beta1.Worker) (scalesim.NodeRunResult, error) {
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
	//totalAllocatableMemory := scaledNode.Status.Allocatable.Memory().MilliValue()
	totalMemoryConsumed := int64(0)
	totalCPUUsage := int64(0)
	totalMemoryCapacity := scaledNode.Status.Capacity.Memory().MilliValue()
	totalCPUCapacity := scaledNode.Status.Capacity.Cpu().MilliValue()
	for _, pod := range targetNodeAssignedPods {
		podMemoryConsumed := int64(0)
		podCPUUsage := int64(0)
		for _, container := range pod.Spec.Containers {
			containerResources, ok := container.Resources.Requests[corev1.ResourceMemory]
			if ok {
				podMemoryConsumed += containerResources.MilliValue()
			}
			containerResources, ok = container.Resources.Requests[corev1.ResourceCPU]
			if ok {
				podCPUUsage += containerResources.MilliValue()
			}
		}
		slog.Info("NodPodAssignment: ", "pod", pod.Name, "node", pod.Spec.NodeName, "memory", podMemoryConsumed, "cpu", podCPUUsage)
		totalMemoryConsumed += podMemoryConsumed
		totalCPUUsage += podCPUUsage
	}

	nodeScore.MemoryWasteRatio = strategy.LeastWaste * (float64(totalMemoryCapacity-totalMemoryConsumed) / float64(totalMemoryCapacity))
	nodeScore.CPUWasteRatio = strategy.LeastWaste * (float64(totalCPUCapacity-totalCPUUsage) / float64(totalCPUCapacity)) //Using the same `LeastWaste` weight. Should we consider having different weights for different resources?
	nodeScore.UnscheduledRatio = float64(len(podListForRun)-totalAssignedPods) / float64(len(podListForRun))
	nodeScore.CostRatio = strategy.LeastCost * getCostRatio(scaledNode, workerPools)
	nodeScore.CumulativeScore = ((nodeScore.MemoryWasteRatio + nodeScore.CPUWasteRatio) / 2) + (nodeScore.UnscheduledRatio * nodeScore.CostRatio)

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
