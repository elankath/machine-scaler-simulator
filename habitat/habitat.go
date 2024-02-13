package habitat

import (
	"fmt"
	"log/slog"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Access struct {
	Client         client.Client
	RestConfig     *rest.Config
	Environment    *envtest.Environment
	KubeconfigPath string
}

func InitHabitat(scheme *runtime.Scheme, binaryAssetsDir string, apiServerFlags map[string]string) (*Access, error) {
	habitatEnv := &envtest.Environment{
		Scheme:                   scheme,
		BinaryAssetsDirectory:    binaryAssetsDir,
		AttachControlPlaneOutput: true,
	}

	if apiServerFlags != nil {
		kubeApiServerArgs := habitatEnv.ControlPlane.GetAPIServer().Configure()
		for k, v := range apiServerFlags {
			kubeApiServerArgs.Set(k, v)
		}
	}

	cfg, err := habitatEnv.Start()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("got nil from envtest.Environment.Start()")
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	kubeConfigPath, err := CreateKubeconfigFileForRestConfig(*cfg)
	if err != nil {
		return nil, err
	}
	slog.Info("Wrote kubeconfig", "kubeconfig", kubeConfigPath)

	return &Access{
		Client:         k8sClient,
		RestConfig:     cfg,
		Environment:    habitatEnv,
		KubeconfigPath: kubeConfigPath,
	}, nil
}

func CreateKubeconfigFileForRestConfig(restConfig rest.Config) (string, error) {
	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   restConfig.Host,
		CertificateAuthorityData: restConfig.CAData,
	}
	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  "default-cluster",
		AuthInfo: "default-user",
	}
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	authinfos["default-user"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: restConfig.CertData,
		ClientKeyData:         restConfig.KeyData,
	}
	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}
	kubeConfigFile, _ := os.Create("/tmp/mcv2poc-kubeconfig.yaml")
	//kubeConfigFile, _ := os.CreateTemp("/tmp/", "kubeconfig")
	err := clientcmd.WriteToFile(clientConfig, kubeConfigFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to create kubeconfig: %w", err)
	}
	return kubeConfigFile.Name(), nil
}
