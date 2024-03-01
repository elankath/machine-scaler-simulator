package simutil

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/elankath/scaler-simulator/virtualcluster"
	"github.com/elankath/scaler-simulator/webutil"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	scalesim "github.com/elankath/scaler-simulator"
)

// GetFailedSchedulingEvents get all FailedSchedulingEvents whose referenced pod does not have a node assigned
// FIXME: This should take a since time.Time which is the scenario start time.
func GetFailedSchedulingEvents(ctx context.Context, a scalesim.VirtualClusterAccess, since time.Time) ([]corev1.Event, error) {
	var failedSchedulingEvents []corev1.Event

	events, err := a.ListEvents(ctx)
	if err != nil {
		slog.Error("cannot list events", "error", err)
		return nil, err
	}
	for _, event := range events {
		sinceTime := metav1.NewTime(since)
		if event.EventTime.BeforeTime(&sinceTime) {
			continue
		}
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

func PrintScheduledPodEvents(ctx context.Context, a scalesim.VirtualClusterAccess, since time.Time, w http.ResponseWriter) error {
	events, err := a.ListEvents(ctx)
	if err != nil {
		slog.Error("cannot list events", "error", err)
		return err
	}
	slices.SortFunc(events, func(a, b corev1.Event) int {
		return cmp.Compare(a.EventTime.UnixMicro(), b.EventTime.Time.UnixMicro())
	})
	for _, event := range events {
		sinceTime := metav1.NewTime(since)
		if event.EventTime.BeforeTime(&sinceTime) {
			continue
		}
		if event.Reason == "Scheduled" { //TODO: find if there a constant for 'FailedScheduling'
			pod, err := a.GetPod(ctx, types.NamespacedName{Name: event.InvolvedObject.Name, Namespace: event.InvolvedObject.Namespace})
			if err != nil {
				return err
			}
			webutil.Log(w, fmt.Sprintf("Pod %s is scheduled on Node %s at time %s", pod.Name, pod.Spec.NodeName, event.EventTime.Time))
		}
	}
	return nil
}

func WaitTillNoUnscheduledPodsOrTimeout(ctx context.Context, access scalesim.VirtualClusterAccess, timeout time.Duration, since time.Time) error {
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
			eventList, err := GetFailedSchedulingEvents(ctx, access, since)
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

func WaitAndGetUnscheduledPodCount(ctx context.Context, access scalesim.VirtualClusterAccess, waitSec int) (int, error) {
	<-time.After(time.Duration(waitSec) * time.Second)
	unscheduledPodCount := 0
	pods, err := access.ListPods(ctx)
	if err != nil {
		return 0, err
	}
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			unscheduledPodCount++
		}
	}
	return unscheduledPodCount, err
}

func GetNodePodAssignments(ctx context.Context, a scalesim.VirtualClusterAccess) ([]scalesim.NodePodAssignment, error) {
	nodes, err := a.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("no scale up done")
	}
	assignMap := make(map[string]scalesim.NodePodAssignment)
	for _, n := range nodes {
		assignMap[n.Name] = scalesim.NodePodAssignment{
			NodeName:        n.Name,
			ZoneName:        n.Labels["topology.kubernetes.io/zone"],
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

func CreateNodeInWorkerGroupForZone(ctx context.Context, a scalesim.VirtualClusterAccess, zone, region string, wg *v1beta1.Worker) (bool, error) {
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

	//TODO: Change this to use the zone and region labels
	//if int32(len(wgNodes)) >= wg.Maximum {
	//	return false, nil
	//}

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
	node.Labels["topology.kubernetes.io/zone"] = zone
	node.Labels["topology.kubernetes.io/region"] = region
	delete(node.Labels, "app.kubernetes.io/existing-node")
	if err := a.AddNodes(ctx, &node); err != nil {
		return false, err
	}
	return true, nil
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
	delete(node.Labels, "app.kubernetes.io/existing-node")
	if err := a.AddNodes(ctx, &node); err != nil {
		return false, err
	}
	return true, nil
}

// CreateNodesTillPoolMax creates sample nodes in the given worker pool till the worker pool max is reached.
func CreateNodesTillPoolMax(ctx context.Context, a scalesim.VirtualClusterAccess, wg *v1beta1.Worker) (int, error) {
	totalNodesCreated := 0
	for i := int32(0); i < wg.Maximum; i++ {
		created, err := CreateNodeInWorkerGroup(ctx, a, wg)
		if err != nil {
			return totalNodesCreated, err
		}
		if !created {
			break
		}
		totalNodesCreated++
	}
	return totalNodesCreated, nil
}

// CreateNodesTillZonexPoolMax creates sample nodes in the given worker pool till the worker pool max is reached.
func CreateNodesTillZonexPoolMax(ctx context.Context, a scalesim.VirtualClusterAccess, region string, wg *v1beta1.Worker) error {
	for _, zone := range wg.Zones {
		for i := int32(0); i < wg.Maximum; i++ {
			created, err := CreateNodeInWorkerGroupForZone(ctx, a, zone, region, wg)
			if err != nil {
				return err
			}
			if !created {
				break
			}
		}
	}
	return nil
}

func GetScalerRecommendation(ctx context.Context, a scalesim.VirtualClusterAccess, assignments []scalesim.NodePodAssignment) (scalesim.ScalerRecommendations, error) {
	recommendation := make(map[string]int)
	nodes, err := a.ListNodes(ctx)
	if err != nil {
		slog.Error("Error getting the nodes", "error", err)
		return recommendation, err
	}

	virtualNodes := make(map[string]corev1.Node)

	for _, node := range nodes {
		if _, ok := node.Labels["app.kubernetes.io/existing-node"]; !ok {
			virtualNodes[node.Name] = node
		}
	}

	for _, assignment := range assignments {
		if _, ok := virtualNodes[assignment.NodeName]; !ok {
			continue
		}
		if len(assignment.PodNameAndCount) > 0 {
			key := fmt.Sprintf("%s/%s", assignment.PoolName, assignment.ZoneName)
			recommendation[key]++
		}
	}

	return recommendation, nil
}

func LogError(w http.ResponseWriter, scenarioName string, err error) {
	webutil.Log(w, "Execution of scenario: "+scenarioName+" completed with error: "+err.Error())
	slog.Error("Execution of scenario: "+scenarioName+" ran into error", "error", err)
}

func ApplyDsPodsToNodes(ctx context.Context, v scalesim.VirtualClusterAccess, dsPods []corev1.Pod) error {
	allNodes, err := v.ListNodes(ctx)
	if err != nil {
		return err
	}

	for _, node := range allNodes {
		if node.Annotations["app.kubernetes.io/existing-node"] != "" {
			continue
		}
		var deployablePods []corev1.Pod
		for _, pod := range dsPods {
			p := *pod.DeepCopy()
			if p.GenerateName != "" {
				p.Name = ""
			}
			p.Spec.NodeName = node.Name
			p.Spec.PriorityClassName = ""
			p.Spec.Priority = nil
			deployablePods = append(deployablePods, p)
		}
		slog.Info("Creating DS pods for node", "node", node.Name, "numPods", len(deployablePods))
		err = v.CreatePods(ctx, virtualcluster.BinPackingSchedulerName, deployablePods...)
		if err != nil {
			return err
		}
	}

	return nil
}
