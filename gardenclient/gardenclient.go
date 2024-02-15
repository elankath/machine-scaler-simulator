package gardenclient

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	scalesim "github.com/elankath/scaler-simulator"
)

type shootAccess struct {
	projectName string
	shootName   string
	getShootCmd *exec.Cmd
	getNodesCmd *exec.Cmd
	codec       runtime.Codec
	////shoot
	//shoot *gardencore.Shoot
	////Nodes
	//nodeList []corev1.Node
}

func (s *shootAccess) ProjectName() string {
	return s.projectName
}

func (s *shootAccess) ShootName() string {
	return s.shootName
}

var (
	codec runtime.Codec
)

var _ scalesim.ShootAccess = (*shootAccess)(nil)

func InitShootAccess(projectName, shootName string) (scalesim.ShootAccess, error) {
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, fmt.Sprintf("KUBECONFIG=%s", os.Getenv("GARDENCTL_KUBECONFIG")))
	cmdEnv = append(cmdEnv, "GCTL_SESSION_ID=scalesim")

	shellCmd := fmt.Sprintf("gardenctl target --garden sap-landscape-dev --project %s >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get shoot %s -oyaml", projectName, shootName)
	getShootCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getShootCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden sap-landscape-dev --project %s --shoot %s >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get node -oyaml", projectName, shootName)
	getNodesCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getNodesCmd.Env = cmdEnv

	configScheme := runtime.NewScheme()
	utilruntime.Must(gardencore.AddToScheme(configScheme))
	utilruntime.Must(corev1.AddToScheme(configScheme))
	ser := json.NewSerializerWithOptions(json.DefaultMetaFactory, configScheme, configScheme, json.SerializerOptions{
		Yaml:   true,
		Pretty: false,
		Strict: false,
	})
	versions := schema.GroupVersions([]schema.GroupVersion{
		gardencore.SchemeGroupVersion,
		corev1.SchemeGroupVersion,
	})
	codec = serializer.NewCodecFactory(configScheme).CodecForVersions(ser, ser, versions, versions)

	return &shootAccess{
		projectName: projectName,
		shootName:   shootName,
		getShootCmd: getShootCmd,
		getNodesCmd: getNodesCmd,
		codec:       codec,
	}, nil
}

func (s *shootAccess) GetShootObj() (*gardencore.Shoot, error) {
	slog.Info("shootAccess.GetShootObj().", "command", s.getShootCmd.String())
	cmdOutput, err := s.getShootCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get shoot obj", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	obj, err := runtime.Decode(s.codec, cmdOutput)
	if err != nil {
		slog.Error("cannot decode command output", "error", err)
		return nil, err
	}
	var shoot = *(obj.(*gardencore.Shoot))
	return &shoot, nil
}

func (s *shootAccess) GetNodes() ([]corev1.Node, error) {
	slog.Info("shootAccess.GetNodes().", "command", s.getNodesCmd.String())
	cmdOutput, err := s.getNodesCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get shoot nodes", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	obj, err := runtime.Decode(s.codec, cmdOutput)
	if err != nil {
		slog.Error("cannot decode command output", "error", err)
		return nil, err
	}
	var list corev1.List
	var nodeList []corev1.Node
	list = *(obj.(*corev1.List))
	for _, item := range list.Items {
		var node corev1.Node
		bytes := item.Raw
		decodedObj, err := runtime.Decode(codec, bytes)
		if err != nil {
			slog.Error("error decoding node", "error", err)
			return nil, err
		}
		node = *(decodedObj.(*corev1.Node))
		nodeList = append(nodeList, node)
	}
	return nodeList, nil
}

func GetShootYaml() (*gardencore.Shoot, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		slog.Error("error fetching present working dir", "error", err)
		return nil, err
	}
	command := exec.Command(workingDir + "/gardenclient/getshoot.sh")

	cmdOutput, err := command.Output()
	if err != nil {
		slog.Error("error running command", "error", err)
		return nil, err
	}

	var shoot gardencore.Shoot

	configScheme := runtime.NewScheme()
	utilruntime.Must(gardencore.AddToScheme(configScheme))
	ser := json.NewSerializerWithOptions(json.DefaultMetaFactory, configScheme, configScheme, json.SerializerOptions{
		Yaml:   true,
		Pretty: false,
		Strict: false,
	})
	versions := schema.GroupVersions([]schema.GroupVersion{
		gardencore.SchemeGroupVersion,
	})
	codec = serializer.NewCodecFactory(configScheme).CodecForVersions(ser, ser, versions, versions)
	obj, err := runtime.Decode(codec, cmdOutput)
	if err != nil {
		slog.Error("error decoding command output", "error", err)
		return nil, err
	}

	shoot = *(obj.(*gardencore.Shoot))

	return &shoot, nil
}

func GetNodeListYaml() ([]corev1.Node, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		slog.Error("error fetching present working dir", "error", err)
		return nil, err
	}
	command := exec.Command(workingDir + "/gardenclient/getnodelist.sh")

	cmdOutput, err := command.Output()
	if err != nil {
		slog.Error("error running command", "error", err)
		return nil, err
	}

	var list corev1.List
	var nodeList []corev1.Node

	configScheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(configScheme))
	ser := json.NewSerializerWithOptions(json.DefaultMetaFactory, configScheme, configScheme, json.SerializerOptions{
		Yaml:   true,
		Pretty: false,
		Strict: false,
	})
	versions := schema.GroupVersions([]schema.GroupVersion{
		corev1.SchemeGroupVersion,
	})
	codec = serializer.NewCodecFactory(configScheme).CodecForVersions(ser, ser, versions, versions)
	obj, err := runtime.Decode(codec, cmdOutput)
	if err != nil {
		slog.Error("error decoding command output", "error", err)
		return nil, err
	}

	list = *(obj.(*corev1.List))
	for _, item := range list.Items {
		var node corev1.Node
		bytes := item.Raw
		decodedObj, err := runtime.Decode(codec, bytes)
		if err != nil {
			slog.Error("error decoding node", "error", err)
			return nil, err
		}
		node = *(decodedObj.(*corev1.Node))
		nodeList = append(nodeList, node)
	}

	return nodeList, nil
}
