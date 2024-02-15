package scalesim

import (
	"context"
	"net/http"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// Engine is the primary simulation driver facade of the scaling simulator. Since Engine register routes for driving simulation scenarios it extends http.Handler
type Engine interface {
	http.Handler
}

// VirtualClusterAccess represents access to the virtualcluster cluster managed by the simulator that shadows the real cluster
type VirtualClusterAccess interface {
	// KubeConfigPath gets path to the kubeconfig.yaml file that can be used by kubectl to connect to this vitual cluster
	KubeConfigPath() string

	// AddNodes adds the given slice of k8s Nodes to the virtual cluster
	AddNodes(ctx context.Context, nodes []corev1.Node) error

	// ClearAll clears all k8s objects from the virtual cluster.
	ClearAll(ctx context.Context) error

	// ClearNodes  clears all nodes from the virtual cluster
	ClearNodes(ctx context.Context) error

	// ClearPods clears all nodes from the virtual cluster
	ClearPods(ctx context.Context) error

	// Shutdown shuts down all components of the virtualcluster cluster. Log all errors encountered during shutdown
	Shutdown()
}

// ShootAccess is a facade to the real-world shoot data and real shoot cluster
type ShootAccess interface {
	// ProjectName returns the project name that the shoot belongs to
	ProjectName() string

	// ShootName returns the name of the shoot
	ShootName() string

	// GetShootObj returns the shoot object describing the shoot cluster
	GetShootObj() (*gardencore.Shoot, error)

	// GetNodes returns slice of nodes of the shoot cluster
	GetNodes() ([]corev1.Node, error)
}
