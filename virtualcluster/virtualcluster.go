package virtualcluster

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	scalesim "github.com/elankath/scaler-simulator"
)

var kubeConfigPath = "/tmp/scalesim-kubeconfig.yaml"

type access struct {
	Client               client.Client
	RestConfig           *rest.Config
	Environment          *envtest.Environment
	KubeSchedulerProcess *os.Process
}

var _ scalesim.VirtualClusterAccess = (*access)(nil) // Verify that *T implements I.

func (a *access) KubeConfigPath() string {
	return kubeConfigPath
}

func InitializeAccess(scheme *runtime.Scheme, binaryAssetsDir string, apiServerFlags map[string]string) (scalesim.VirtualClusterAccess, error) {
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

	err = CreateKubeconfigFileForRestConfig(*cfg)
	if err != nil {
		return nil, err
	}
	slog.Info("Wrote kubeconfig", "kubeconfig", kubeConfigPath)

	schedulerProcess, err := StartScheduler(binaryAssetsDir)
	access := &access{
		Client:               k8sClient,
		RestConfig:           cfg,
		Environment:          habitatEnv,
		KubeSchedulerProcess: schedulerProcess,
	}
	if err != nil {
		slog.Info("cannot start kube-scheduler.", "error", err)
		_ = access.Shutdown()
		return nil, err
	}
	return access, nil
}

func (a *access) Shutdown() (err error) {
	err = a.Environment.Stop()
	if err != nil {
		return err
	}
	if a.KubeSchedulerProcess != nil { //TODO: launch kube-scheduler as part of simulator.
		err = a.KubeSchedulerProcess.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}
		slog.Info("waiting for kube-scheduler to exit...", "signal", syscall.SIGTERM.String())
		procState, err := a.KubeSchedulerProcess.Wait()
		slog.Info("kube-scheduler done", "exited", procState.Exited(), "exit-success", procState.Success(), "exit-code", procState.ExitCode())
		return err
	}
	return
}

func CreateKubeconfigFileForRestConfig(restConfig rest.Config) error {
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
	kubeConfigFile, _ := os.Create(kubeConfigPath)
	//kubeConfigFile, _ := os.CreateTemp("/tmp/", "kubeconfig")
	err := clientcmd.WriteToFile(clientConfig, kubeConfigFile.Name())
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig: %w", err)
	}
	return nil
}

func StartScheduler(binaryAssetsDir string) (*os.Process, error) {
	//wd, err := os.Getwd()
	//if err != nil {
	//	return nil, fmt.Errorf("cannot get working dir: %w", err)
	//}
	command := exec.Command(binaryAssetsDir+"/kube-scheduler", "--kubeconfig", kubeConfigPath, "--leader-elect=false")
	command.Stderr = os.Stderr
	command.Stdout = os.Stdout
	slog.Info("launching kube-scheduler", "command", command)
	err := command.Start()
	if err != nil {
		return nil, fmt.Errorf("cannot start kube-scheduler: %w", err)
	}
	return command.Process, nil
}
