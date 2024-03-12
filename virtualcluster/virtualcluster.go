package virtualcluster

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/serutil"
)

var kubeConfigPath = "/tmp/scalesim-kubeconfig.yaml"

const BinPackingSchedulerName = "bin-packing-scheduler"

type access struct {
	client               client.Client
	restConfig           *rest.Config
	environment          *envtest.Environment
	kubeSchedulerProcess *os.Process
}

var _ scalesim.VirtualClusterAccess = (*access)(nil) // Verify that *T implements I.

func (a *access) UpdatePods(ctx context.Context, pods ...corev1.Pod) error {
	for _, pod := range pods {
		err := a.client.Update(ctx, &pod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *access) DeleteNode(ctx context.Context, name string) error {
	node := corev1.Node{}
	err := a.client.Get(ctx, types.NamespacedName{Name: name}, &node)
	if err != nil {
		return err
	}
	err = a.client.Delete(ctx, &node)
	if err != nil {
		return err
	}
	return nil
}
func (a *access) CreatePods(ctx context.Context, schedulerName string, pods ...corev1.Pod) error {
	for _, pod := range pods {
		var podObjMeta metav1.ObjectMeta
		if pod.GenerateName != "" {
			podObjMeta = metav1.ObjectMeta{
				GenerateName:    pod.GenerateName,
				Namespace:       pod.Namespace,
				OwnerReferences: pod.OwnerReferences,
				Labels:          pod.Labels,
				Annotations:     pod.Annotations,
			}
		} else {
			podObjMeta = metav1.ObjectMeta{
				Name:            pod.Name,
				Namespace:       pod.Namespace,
				OwnerReferences: pod.OwnerReferences,
				Labels:          pod.Labels,
				Annotations:     pod.Annotations,
			}
		}
		dupPod := corev1.Pod{
			ObjectMeta: podObjMeta,
			Spec:       pod.Spec,
		}
		dupPod.Spec.SchedulerName = schedulerName
		dupPod.Spec.TerminationGracePeriodSeconds = ptr.To(int64(0))
		err := a.client.Create(ctx, &dupPod)
		if err != nil {
			slog.Error("Error creating the pod.", "error", err)
			return err
		}
	}
	return nil
}

func (a *access) CreatePodsFromYaml(ctx context.Context, podYamlPath string, replicas int) error {
	pod, err := serutil.ReadPod(podYamlPath)
	if err != nil {
		return fmt.Errorf("cannot read pod spec %q: %w", podYamlPath, err)
	}
	for i := 0; i < replicas; i++ {
		err = a.CreatePods(ctx, BinPackingSchedulerName, pod)
		if err != nil {
			return fmt.Errorf("cannot create replica %d of pod spec %q: %w", i, podYamlPath, err)
		}
	}
	return nil
}

func (a *access) KubeConfigPath() string {
	return kubeConfigPath
}

func InitializeAccess(scheme *runtime.Scheme, binaryAssetsDir string, apiServerFlags map[string]string) (scalesim.VirtualClusterAccess, error) {
	env := &envtest.Environment{
		Scheme:                   scheme,
		BinaryAssetsDirectory:    binaryAssetsDir,
		AttachControlPlaneOutput: true,
	}

	if apiServerFlags != nil {
		kubeApiServerArgs := env.ControlPlane.GetAPIServer().Configure()
		for k, v := range apiServerFlags {
			kubeApiServerArgs.Set(k, v)
		}
	}

	cfg, err := env.Start()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("got nil from envtest.environment.Start()")
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	err = createKubeconfigFileForRestConfig(*cfg)
	if err != nil {
		return nil, err
	}
	slog.Info("Wrote kubeconfig", "kubeconfig", kubeConfigPath)

	schedulerProcess, err := StartScheduler(binaryAssetsDir)
	access := &access{
		client:               k8sClient,
		restConfig:           cfg,
		environment:          env,
		kubeSchedulerProcess: schedulerProcess,
	}
	if err != nil {
		slog.Info("cannot start kube-scheduler.", "error", err)
		access.Shutdown()
		return nil, err
	}
	return access, nil
}

func (a *access) Shutdown() {
	slog.Info("STOPPING env test apiserver,etcd")
	err := a.environment.Stop()
	slog.Warn("error stopping envtest.", "error", err)
	if a.kubeSchedulerProcess != nil { //TODO: launch kube-scheduler as part of simulator.
		err = a.kubeSchedulerProcess.Signal(syscall.SIGTERM)
		if err != nil {
			slog.Warn("error signalling kubescheduler process.", "error", err)
		}
		slog.Info("waiting for kube-scheduler to exit...", "signal", syscall.SIGTERM.String())
		procState, err := a.kubeSchedulerProcess.Wait()
		slog.Info("kube-scheduler done", "exited", procState.Exited(), "exit-success", procState.Success(), "exit-code", procState.ExitCode(), "error", err)
	}
	return
}

func (a *access) AddNodes(ctx context.Context, nodes ...*corev1.Node) error {
	for _, n := range nodes {
		n.ObjectMeta.ResourceVersion = ""
		n.ObjectMeta.UID = ""
		err := a.client.Create(ctx, n)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *access) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := corev1.NodeList{}
	if err := a.client.List(ctx, &nodeList); err != nil {
		slog.Error("cannot list nodes", "error", err)
		return nil, err
	}
	return nodeList.Items, nil
}

func (a *access) ListPods(ctx context.Context) ([]corev1.Pod, error) {
	podList := corev1.PodList{}
	if err := a.client.List(ctx, &podList); err != nil {
		slog.Error("cannot list pods", "error", err)
		return nil, err
	}
	return podList.Items, nil
}

func (a *access) GetPod(ctx context.Context, fullName types.NamespacedName) (*corev1.Pod, error) {
	pod := corev1.Pod{}
	err := a.client.Get(ctx, fullName, &pod)
	return &pod, err
}

func (a *access) ListEvents(ctx context.Context) ([]corev1.Event, error) {
	eventList := corev1.EventList{}
	if err := a.client.List(ctx, &eventList); err != nil {
		slog.Error("error list events", "error", err)
		return nil, err
	}
	return eventList.Items, nil
}

func (a *access) RemoveTaintFromVirtualNodes(ctx context.Context) error {
	nodeList := corev1.NodeList{}
	if err := a.client.List(ctx, &nodeList); err != nil {
		slog.Error("error list nodes", "error", err)
		return err
	}

	for _, no := range nodeList.Items {
		if _, ok := no.Labels["app.kubernetes.io/existing-node"]; ok {
			continue
		}
		node := corev1.Node{}
		if err := a.client.Get(ctx, client.ObjectKeyFromObject(&no), &node); err != nil {
			return err
		}
		patch := client.MergeFrom(node.DeepCopy())
		node.Spec.Taints = nil

		if err := a.client.Patch(ctx, &node, patch); err != nil {
			return err
		}
	}
	return nil
}

func (a *access) ApplyK8sObject(ctx context.Context, k8sObjs ...runtime.Object) error {
	for _, obj := range k8sObjs {
		switch obj.(type) { //TODO: add more cases here as and when needed
		case *corev1.Pod:
			pod := obj.(*corev1.Pod)
			pod.ObjectMeta.ResourceVersion = ""
			pod.ObjectMeta.UID = ""
			err := a.client.Create(ctx, pod)
			if err != nil {
				return err
			}
		case *appsv1.Deployment:
			deploy := obj.(*appsv1.Deployment)
			err := a.client.Create(ctx, deploy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *access) ClearAll(ctx context.Context) (err error) {
	// kubectl delete all --all
	var errBuffer bytes.Buffer
	delCmd := exec.Command("kubectl", "--kubeconfig", kubeConfigPath, "delete", "all", "--all")
	delCmd.Stderr = &errBuffer
	out, err := delCmd.Output()
	if err != nil {
		slog.Error("failed to clear all objects.", "command", delCmd, "stderr", string(errBuffer.Bytes()))
		return fmt.Errorf("failed to clear objects: %w", err)
	}
	err = a.ClearNodes(ctx)
	if err != nil {
		return err
	}
	<-time.After(2 * time.Second)
	slog.Info("cleared all objects", "command", delCmd, "stdout", string(out))
	return
}

func (a *access) ClearNodes(ctx context.Context) error {
	nodeList := &corev1.NodeList{}
	err := a.client.List(ctx, nodeList)
	if err != nil {
		return fmt.Errorf("cannot list pods: %w", err)
	}
	for i := 0; i < len(nodeList.Items); i++ {
		n := &nodeList.Items[i]
		slog.Info("deleting node.", "name", n.Name)
		err = a.client.Delete(ctx, n)
		if err != nil {
			return fmt.Errorf("cannot delete node %q: %w", n.Name, err)
		}
	}
	return nil
}

func (a *access) ClearPods(ctx context.Context) error {
	podList := &corev1.PodList{}
	err := a.client.List(ctx, podList)
	if err != nil {
		return fmt.Errorf("cannot list pods: %w", err)
	}
	for i := 0; i < len(podList.Items); i++ {
		p := &podList.Items[i]
		slog.Info("deleting pod.", "name", p.Name)
		err = a.client.Delete(ctx, p)
		if err != nil {
			return fmt.Errorf("cannot delete pod %q: %w", p.Name, err)
		}
	}
	return nil
}

func (a *access) TrimCluster(ctx context.Context) error {
	nonEmptyNodes := make(map[string]struct{})

	podList, err := a.ListPods(ctx)
	if err != nil {
		return err
	}

outer:
	for _, pod := range podList {
		for _, ownRef := range pod.OwnerReferences {
			if ownRef.Kind == "DaemonSet" {
				continue outer
			}
		}
		if pod.Spec.NodeName != "" {
			nonEmptyNodes[pod.Spec.NodeName] = struct{}{}
		}
	}

	nodes, err := a.ListNodes(ctx)
	if err != nil {
		return err
	}

	for _, n := range nodes {
		_, ok := nonEmptyNodes[n.Name]
		if ok {
			continue
		}
		slog.Info("deleting node from cluster", "nodeName", n.Name)
		err = a.client.Delete(ctx, &n)
		if err != nil {
			return err
		}
	}

	//TODO: make this logic better. probably allow configuration.
	for _, pod := range podList {
		_, ok := nonEmptyNodes[pod.Spec.NodeName]
		if !ok {
			slog.Info("deleting pod from cluster", "podName", pod.Name, "nodeName", pod.Spec.NodeName)
			err = a.client.Delete(ctx, &pod)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func createKubeconfigFileForRestConfig(restConfig rest.Config) error {
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
	workingDir, _ := os.Getwd()
	schedulerConfigPath := workingDir + "/virtualcluster/scheduler-config.yaml"
	command := exec.Command(binaryAssetsDir+"/kube-scheduler", "--kubeconfig", kubeConfigPath, "--config", schedulerConfigPath, "--leader-elect=false", "--v=3")
	//command := exec.Command(binaryAssetsDir+"/kube-scheduler", "--kubeconfig", kubeConfigPath)
	command.Stderr = os.Stderr
	command.Stdout = os.Stdout
	slog.Info("launching kube-scheduler", "command", command)
	err := command.Start()
	if err != nil {
		return nil, fmt.Errorf("cannot start kube-scheduler: %w", err)
	}
	return command.Process, nil
}
