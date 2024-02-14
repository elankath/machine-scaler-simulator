package gardenclient

import (
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
)

type Model struct {
	//shoot
	shoot *gardencore.Shoot
	//Nodes
	nodeList []corev1.Node
}

var (
	codec runtime.Codec
)

func Connect() *Model {
	shoot, err := GetShootYaml()
	if err != nil {
		return nil
	}
	nodeList, err := GetNodeListYaml()
	if err != nil {
		return nil
	}

	return &Model{
		shoot:    shoot,
		nodeList: nodeList,
	}
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
