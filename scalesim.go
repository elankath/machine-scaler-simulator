// Package scalesim contains the API interface and structural types for the scaler simular project
package scalesim

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Engine is the primary simulation driver facade of the scaling simulator. Since Engine register routes for driving simulation scenarios it extends http.Handler
type Engine interface {
	http.Handler
	VirtualClusterAccess() VirtualClusterAccess
	ShootAccess(shootName string) ShootAccess
	SyncVirtualNodesWithShoot(ctx context.Context, shootName string) error
	ScaleWorkerPoolsTillMaxOrNoUnscheduledPods(ctx context.Context, scenarioName string, since time.Time, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error)
	ScaleAllWorkerPoolsTillMax(ctx context.Context, scenarioName string, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error)
	ScaleWorkerPoolsTillNumZonesMultPoolsMax(ctx context.Context, scenarioName string, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error)
	ScaleWorkerPoolTillMax(ctx context.Context, scenarioName string, pool *gardencore.Worker, w http.ResponseWriter) (int, error)
}

// VirtualClusterAccess represents access to the virtualcluster cluster managed by the simulator that shadows the real cluster
type VirtualClusterAccess interface {
	// KubeConfigPath gets path to the kubeconfig.yaml file that can be used by kubectl to connect to this vitual cluster
	KubeConfigPath() string

	// AddNodes adds the given slice of k8s Nodes to the virtual cluster
	AddNodes(context.Context, ...*corev1.Node) error

	// RemoveTaintFromVirtualNodes removes the NoSchedule taint from all nodes in the virtual cluster
	RemoveTaintFromVirtualNodes(context.Context) error

	// RemoveTaintFromVirtualNode removes the NoSchedule taint from the given node in the virtual cluster
	RemoveTaintFromVirtualNode(context.Context, string) error

	// AddTaintToNode adds the NoSchedule taint from the given node in the virtual cluster
	AddTaintToNode(context.Context, *corev1.Node) error

	// CreatePods creates the given slice of k8s Pods in the virtual cluster
	CreatePods(context.Context, string, ...corev1.Pod) error

	CreatePodsWithNodeAndScheduler(context.Context, string, string, ...corev1.Pod) error

	// CreatePodsFromYaml loads the pod yaml at the given podYamlPath and creates Pods for given number of replicas.
	CreatePodsFromYaml(ctx context.Context, podYamlPath string, replicas int) error

	// ApplyK8sObject applies all Objects into the virtual cluster
	ApplyK8sObject(context.Context, ...runtime.Object) error

	// ClearAll clears all k8s objects from the virtual cluster.
	ClearAll(ctx context.Context) error

	// ClearNodes  clears all nodes from the virtual cluster
	ClearNodes(context.Context) error

	// UpdateNodes updated nodes in the virtual cluster
	UpdateNodes(ctx context.Context, nodes ...corev1.Node) error

	// ClearPods clears all nodes from the virtual cluster
	ClearPods(context.Context) error

	// Shutdown shuts down all components of the virtualcluster cluster. Log all errors encountered during shutdown
	Shutdown()

	GetPod(ctx context.Context, fullName types.NamespacedName) (*corev1.Pod, error)
	ListEvents(cts context.Context) ([]corev1.Event, error)
	ListNodes(ctx context.Context) ([]corev1.Node, error)
	ListNodesInNodePool(ctx context.Context, nodePoolName string) ([]corev1.Node, error)

	// ListPods lists all pods from all namespaces
	ListPods(ctx context.Context) ([]corev1.Pod, error)

	// TrimCluster deletes unused nodes and daemonset pods on these nodes
	TrimCluster(ctx context.Context) error

	UpdatePods(ctx context.Context, pods ...corev1.Pod) error

	DeleteNode(ctx context.Context, name string) error

	DeletePods(ctx context.Context, pods ...corev1.Pod) error

	DeleteNodesWithMatchingLabels(ctx context.Context, labels map[string]string) error

	DeletePodsWithMatchingLabels(ctx context.Context, labels map[string]string) error

	ListNodesMatchingLabels(ctx context.Context, labels map[string]string) ([]corev1.Node, error)
}

// ShootAccess is a facade to the real-world shoot data and real shoot cluster
type ShootAccess interface {
	// ProjectName returns the project name that the shoot belongs to
	ProjectName() string

	// GetShootObj returns the shoot object describing the shoot cluster
	GetShootObj() (*gardencore.Shoot, error)

	// GetNodes returns slice of nodes of the shoot cluster
	GetNodes() ([]*corev1.Node, error)

	// GetUnscheduledPods returns slice of unscheduled pods of the shoot cluster
	GetUnscheduledPods() ([]corev1.Pod, error)

	GetDSPods() ([]corev1.Pod, error)

	// GetMachineDeployments returns slice of machine deployments of the shoot cluster
	GetMachineDeployments() ([]*machinev1alpha1.MachineDeployment, error)

	// ScaleMachineDeployment scales the given machine deployment to the given number of replicas
	ScaleMachineDeployment(machineDeploymentName string, replicas int32) error

	// CreatePods creates the given slice of k8s Pods in the shoot cluster
	CreatePods(filePath string, replicas int) error

	TaintNodes() error

	UntaintNodes() error

	DeleteAllPods() error

	CleanUp() error
}

// Scenario represents a scaling simulation scenario. Each scenario is invocable by an HTTP endpoint and hence extends http.Handler
type Scenario interface {
	http.Handler
	Description() string
	// Name is the name of this scenario
	Name() string
	//ShootName is the name of the shoot that the scenario executes against
	ShootName() string
}

// NodePool describes a worker pool in the shoot.
type NodePool struct {
	Name        string
	Zones       []string
	Max         int32
	Current     int32
	MachineType string
}

type NodePodAssignment struct {
	NodeName     string   `json:"nodeName"`
	ZoneName     string   `json:"zoneName"`
	PoolName     string   `json:"poolName"`
	InstanceType string   `json:"instanceType"`
	PodNames     []string `json:"podNames"`
}

func (n NodePodAssignment) String() string {
	var sb strings.Builder
	sb.WriteString("(Node: " + n.NodeName + " ,Zone: " + n.ZoneName + ", PoolName: " + n.PoolName + ", InstanceType: " + n.InstanceType + ", PodAssignments: ")
	for _, v := range n.PodNames {
		sb.WriteString("[")
		sb.WriteString(v)
		sb.WriteString("],")
	}
	sb.WriteString(")")
	return sb.String()
}

type ScalerRecommendations map[string]int

func (s ScalerRecommendations) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	for k, v := range s {
		sb.WriteString(k + ":" + strconv.Itoa(v) + ",")
	}
	sb.WriteString("}")
	return sb.String()
}

type NodeRunResult struct {
	NodeName         string
	Pool             *gardencore.Worker
	WasteRatio       float64
	UnscheduledRatio float64
	CostRatio        float64
	CumulativeScore  float64
	//NumAssignedPods  int
	NumAssignedPodsToNode int
	NumAssignedPodsTotal  int
}

func (n NodeRunResult) String() string {
	return fmt.Sprintf("(Worker: %s, WasteRatio: %.4f, UnscheduledRatio: %.4f, CostRatio: %.4f, CumulativeScore: %.4f, NumAssignedPodsToNode: %d, NumAssignedPodsTotal: %d)", n.Pool.Name, n.WasteRatio, n.UnscheduledRatio, n.CostRatio, n.CumulativeScore, n.NumAssignedPodsToNode, n.NumAssignedPodsTotal)
}

type AllPricing struct {
	Results []InstancePricing `json:"results"`
}

type InstancePricing struct {
	InstanceType string       `json:"instance_type"`
	VCPU         float64      `json:"vcpu"`
	Memory       float64      `json:"memory"`
	EDPPrice     PriceDetails `json:"edp_price"`
}

type PriceDetails struct {
	PayAsYouGo    float64 `json:"pay_as_you_go"`
	Reserved1Year float64 `json:"ri_1_year"`
	Reserved3Year float64 `json:"ri_3_years"`
}

type NodeRunResults map[string]NodeRunResult

func (ns NodeRunResults) GetWinner() NodeRunResult {
	var winner NodeRunResult
	minScore := math.MaxFloat64
	for _, v := range ns {
		if v.CumulativeScore < minScore {
			winner = v
			minScore = v.CumulativeScore
		}
	}
	return winner
}

func (ns NodeRunResults) GetTotalAssignedPods() int {
	var total int
	for _, v := range ns {
		total += v.NumAssignedPodsToNode
	}
	return total
}

type StrategyWeights struct {
	LeastWaste float64
	LeastCost  float64
}

type Recommendations map[string]*Recommendation

type Recommender struct {
	Engine          Engine
	ScenarioName    string
	ShootName       string
	StrategyWeights StrategyWeights
	LogWriter       http.ResponseWriter
}

type Recommendation struct {
	WorkerPoolName string
	Replicas       int
	Cost           float64
	Waste          resource.Quantity
	Allocatable    resource.Quantity
}

func (r Recommendations) String() string {
	totalCost := 0.0
	totalWaste := resource.NewQuantity(0, resource.BinarySI)
	totalAllocatable := resource.NewQuantity(0, resource.BinarySI)
	var sb strings.Builder
	sb.WriteString("\n")
	for _, v := range r {
		totalCost += v.Cost
		totalWaste.Add(v.Waste)
		totalAllocatable.Add(v.Allocatable)
		wr := float64(v.Waste.MilliValue()) / float64(v.Allocatable.MilliValue())
		_, err := sb.WriteString(fmt.Sprintf("PoolName: %s, Replicas: %d, Cost: %.4f, WasteRatio: %.4f\n", v.WorkerPoolName, v.Replicas, v.Cost, wr))
		if err != nil {
			slog.Error("Error while writing to string builder", "error", err)
		}
	}
	totalWasteRatio := float64(totalWaste.MilliValue()) / float64(totalAllocatable.MilliValue())
	sb.WriteString(fmt.Sprintf("TotalCost: %.4f, TotalWaste: %s, TotalAllocatable: %s, TotalWasteRatio: %.4f", totalCost, totalWaste.String(), totalAllocatable.String(), totalWasteRatio))
	return sb.String()
}
