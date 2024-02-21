package simutil

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	scalesim "github.com/elankath/scaler-simulator"
)

// GetFailedSchedulingEvents get all FailedSchedulingEvents whose referenced pod does not have a node assigned
// FIXME: This should take a since time.Time which is the scenario start time.
func GetFailedSchedulingEvents(ctx context.Context, a scalesim.VirtualClusterAccess) ([]corev1.Event, error) {
	var failedSchedulingEvents []corev1.Event

	events, err := a.ListEvents(ctx)
	if err != nil {
		slog.Error("cannot list events", "error", err)
		return nil, err
	}
	for _, event := range events {
		if event.Reason == "FailedScheduling" { //TODO: find if there a constant for 'FailedScheduling'
			pod, err := a.GetPod(ctx, types.NamespacedName{Name: event.InvolvedObject.Name, Namespace: event.InvolvedObject.Namespace})
			if err != nil {
				return nil, err
			}
			// TODO:(verify with others) have to do have this check since the 'FailedScheduling' events are not deleted
			if pod.Spec.NodeName != "" {
				continue
			}
			failedSchedulingEvents = append(failedSchedulingEvents, event)
		}
	}
	return failedSchedulingEvents, nil
}

func WaitTillNoUnscheduledPodsOrTimeout(ctx context.Context, access scalesim.VirtualClusterAccess, timeout time.Duration) error {
	pollSecs := 2
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			msg := "timeout waiting for unscheduled pods to get scheduled."
			slog.Error(msg, "timeout", timeout, "error", ctx.Err())
			return fmt.Errorf(msg+": %w", ctx.Err())
		default:
			eventList, err := GetFailedSchedulingEvents(ctx, access)
			if err != nil {
				return fmt.Errorf("cant get failed scheduling events due to: w", err)
			}
			if len(eventList) == 0 {
				slog.Info("no FailedScheduling events present.")
				return nil
			}
			slog.Info("wait before polling..", "waitSecs", 2)
			<-time.After(time.Duration(pollSecs) * time.Second)
		}
	}
}

func GetNodePodAssignments(ctx context.Context, a scalesim.VirtualClusterAccess) ([]scalesim.NodePodAssignment, error) {
	nodes, err := a.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	assignMap := make(map[string]scalesim.NodePodAssignment)
	for _, n := range nodes {
		assignMap[n.Name] = scalesim.NodePodAssignment{
			NodeName:        n.Name,
			PoolName:        n.Labels["worker.gardener.cloud/pool"],
			InstanceType:    n.Labels["node.kubernetes.io/instance-type"],
			PodNameAndCount: make(map[string]int),
		}
	}
	pods, err := a.ListPods(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pods {
		nodeName := p.Spec.NodeName
		if nodeName == "" {
			continue
		}
		podNameCategory := p.ObjectMeta.GenerateName
		a := assignMap[nodeName]
		a.PodNameAndCount[podNameCategory]++
	}
	for n, a := range assignMap {
		if len(a.PodNameAndCount) == 0 {
			delete(assignMap, n)
		}
	}
	assignments := maps.Values(assignMap)
	slices.SortFunc(assignments, func(a, b scalesim.NodePodAssignment) int {
		return cmp.Compare(a.PoolName, b.PoolName)
	})
	return assignments, nil
}

// CreateNodeInWorkerGroup creates a sample node if the passed workerGroup objects max has not been met
func CreateNodeInWorkerGroup(ctx context.Context, a scalesim.VirtualClusterAccess, wg *v1beta1.Worker) (bool, error) {
	nodes, err := a.ListNodes(ctx)
	if err != nil {
		return false, err
	}

	var wgNodes []corev1.Node
	for _, n := range nodes {
		if n.Labels["worker.garden.sapcloud.io/group"] == wg.Name {
			wgNodes = append(wgNodes, n)
		}
	}

	if int32(len(wgNodes)) >= wg.Maximum {
		return false, nil
	}

	var deployedNode *corev1.Node
	for _, node := range wgNodes {
		if node.Labels["worker.garden.sapcloud.io/group"] == wg.Name {
			deployedNode = &node
		}
	}

	if deployedNode == nil {
		return false, nil
	}
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", wg.Name),
			Namespace:    "default",
			//TODO Change k8s hostname labels
			Labels: deployedNode.Labels,
		},
		Status: corev1.NodeStatus{
			Allocatable: deployedNode.Status.Allocatable,
			Capacity:    deployedNode.Status.Capacity,
			Phase:       corev1.NodeRunning,
		},
	}
	if err := a.AddNodes(ctx, node); err != nil {
		return false, err
	}
	return true, nil
}

// CreateNodesTillMax creates sample nodes in the given worker pool till the worker pool max is reached.
func CreateNodesTillMax(ctx context.Context, a scalesim.VirtualClusterAccess, wg *v1beta1.Worker) error {
	for i := int32(0); i < wg.Maximum; i++ {
		created, err := CreateNodeInWorkerGroup(ctx, a, wg)
		if err != nil {
			return err
		}
		if !created {
			break
		}
	}
	return nil
}
