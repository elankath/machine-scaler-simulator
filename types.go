package scalesim

import (
	"context"
	"net/http"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Engine is the primary simulation driver facade of the scaling simulator. Since Engine register routes for driving simulation scenarios it extends http.Handler
type Engine interface {
	http.Handler

	VirtualClusterAccess() VirtualClusterAccess
	ShootAccess(shootName string) ShootAccess
	SyncNodes(ctx context.Context, shootName string) error
	ApplyPod(ctx context.Context, podSpecPath string, replicas int, waitSecs int) error
	ScaleWorkerPoolsTillMaxOrNoUnscheduledPods(ctx context.Context, scenarioName string, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error)
	ScaleAllWorkerPoolsTillMax(ctx context.Context, scenarioName string, shoot *gardencore.Shoot, w http.ResponseWriter) (int, error)
}

// VirtualClusterAccess represents access to the virtualcluster cluster managed by the simulator that shadows the real cluster
type VirtualClusterAccess interface {
	// KubeConfigPath gets path to the kubeconfig.yaml file that can be used by kubectl to connect to this vitual cluster
	KubeConfigPath() string

	// AddNodes adds the given slice of k8s Nodes to the virtual cluster
	AddNodes(context.Context, []corev1.Node) error

	// RemoveTaintFromNode removed the NoSchedule taint from all nodes in the virtual cluster
	RemoveTaintFromNode(context.Context) error

	// CreatePods creates the given slice of k8s Pods in the virtual cluster
	CreatePods(context.Context, ...corev1.Pod) error

	// ApplyK8sObject applies all Objects into the virtual cluster
	ApplyK8sObject(context.Context, ...runtime.Object) error

	// GetFailedSchedulingEvents get all FailedSchedulingEvents whose referenced pod does not have a node assigned
	GetFailedSchedulingEvents(context.Context) ([]corev1.Event, error)

	// CreateNodeInWorkerGroup creates a sample node if the passed workerGroup objects max has not been met
	CreateNodeInWorkerGroup(context.Context, *v1beta1.Worker) (bool, error)

	// CreateNodesTillMax creates sample nodes in the given worker pool till the worker pool max is reached.
	CreateNodesTillMax(context.Context, *v1beta1.Worker) error

	// ClearAll clears all k8s objects from the virtual cluster.
	ClearAll(ctx context.Context) error

	// ClearNodes  clears all nodes from the virtual cluster
	ClearNodes(context.Context) error

	// ClearPods clears all nodes from the virtual cluster
	ClearPods(context.Context) error

	// Shutdown shuts down all components of the virtualcluster cluster. Log all errors encountered during shutdown
	Shutdown()

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
	GetNodes() ([]corev1.Node, error)
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
