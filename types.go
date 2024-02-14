package scalesim

import (
	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// VirtualClusterAccess represents access to the virtualclient cluster managed by the simulator that shadows the real cluster
type VirtualClusterAccess interface {
	// KubeConfigPath gets path to the kubeconfig.yaml file that can be used by kubectl to connect to this vitual cluster
	KubeConfigPath() string

	// Shutdown shuts down all components of the virtualclient cluster. Returns err encountered during shutdown if any.
	Shutdown() error

	//TODO: CreateNodes
	//TODO: CreatePods
	//TODO: ClearAll()
	//TODO: Restart()

	// RestConfig method returns the client-go rest.Config that can be used by clients to connect to the virtualclient cluster.
	// No, Dont expose this. use high-level operations to abstract functionality.
	//RestConfig() *rest.Config
}

// ShootAccess is a facade to the real-world shoot data and real shoot cluster
type ShootAccess interface {
	// GetShootObj returns the shoot object describing the shoot cluster
	GetShootObj() (*gardencore.Shoot, error)

	// GetNodes returns slice of nodes of the shoot cluster
	GetNodes() ([]corev1.Node, error)
}
