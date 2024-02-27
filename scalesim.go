// Package scalesim contains the API interface and structural types for the scaler simular project
package scalesim

import (
	"context"
	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// RemoveTaintFromVirtualNodes removed the NoSchedule taint from all nodes in the virtual cluster
	RemoveTaintFromVirtualNodes(context.Context) error

	// CreatePods creates the given slice of k8s Pods in the virtual cluster
	CreatePods(context.Context, ...corev1.Pod) error

	// CreatePodsFromYaml loads the pod yaml at the given podYamlPath and creates Pods for given number of replicas.
	CreatePodsFromYaml(ctx context.Context, podYamlPath string, replicas int) error

	// ApplyK8sObject applies all Objects into the virtual cluster
	ApplyK8sObject(context.Context, ...runtime.Object) error

	// ClearAll clears all k8s objects from the virtual cluster.
	ClearAll(ctx context.Context) error

	// ClearNodes  clears all nodes from the virtual cluster
	ClearNodes(context.Context) error

	// ClearPods clears all nodes from the virtual cluster
	ClearPods(context.Context) error

	// Shutdown shuts down all components of the virtualcluster cluster. Log all errors encountered during shutdown
	Shutdown()

	GetPod(ctx context.Context, fullName types.NamespacedName) (*corev1.Pod, error)
	ListEvents(cts context.Context) ([]corev1.Event, error)
	ListNodes(ctx context.Context) ([]corev1.Node, error)

	ListPods(ctx context.Context) ([]corev1.Pod, error)
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

	// GetMachineDeployments returns slice of machine deployments of the shoot cluster
	GetMachineDeployments() ([]*machinev1alpha1.MachineDeployment, error)

	// ScaleMachineDeployment scales the given machine deployment to the given number of replicas
	ScaleMachineDeployment(machineDeploymentName string, replicas int32) error

	// CreatePods creates the given slice of k8s Pods in the shoot cluster
	CreatePods(filePath string, replicas int) error
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

type NodePodAssignment struct {
	NodeName        string
	ZoneName        string
	PoolName        string
	InstanceType    string
	PodNameAndCount map[string]int
}

func (n NodePodAssignment) String() string {
	var sb strings.Builder
	sb.WriteString("(Node: " + n.NodeName + " ,Zone: " + n.ZoneName + ", PoolName: " + n.PoolName + ", InstanceType: " + n.InstanceType + ", PodAssignments: ")
	for k, v := range n.PodNameAndCount {
		sb.WriteString("[")
		sb.WriteString(k + ":" + strconv.Itoa(v))
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
