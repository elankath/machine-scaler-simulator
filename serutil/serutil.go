package serutil

import (
	"log/slog"
	"os"
	"strings"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var codec runtime.Codec
var workingDir string

func init() {
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
	workingDir, err := os.Getwd()
	if err != nil {
		slog.Error("no working dir found", "error", err)
		os.Exit(9)
	}
	slog.Info("working dir", "workingDir", workingDir)
}

// ConstructK8sObject reads a yaml file from given relative Path containing k8s resources and creates objects
func ConstructK8sObject(projRelPath string) ([]runtime.Object, error) {
	workingDir, _ := os.Getwd() // FIXME: package var not accessible for some reason.
	path := workingDir + "/" + projRelPath
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	stringFromBytes := string(bytes)

	var ObjectList []runtime.Object
	for _, podSpec := range strings.Split(stringFromBytes, "---") {
		if len(podSpec) == 0 {
			continue
		}
		obj, err := runtime.Decode(codec, []byte(podSpec))
		if err != nil {
			slog.Error("cannot decode object", "error", err)
			return nil, err
		}
		ObjectList = append(ObjectList, obj)
	}

	return ObjectList, nil
}

// DecodeShoot unmarshalls the given byte slice as a garden shoot obj . TODO: Fix this to use generics later
func DecodeShoot(bytes []byte) (*gardencore.Shoot, error) {
	obj, err := runtime.Decode(codec, bytes)
	if err != nil {
		slog.Error("cannot decode as shoot obj", "error", err)
		return nil, err
	}
	var shoot = obj.(*gardencore.Shoot)
	return shoot, nil
}

func DecodeNodeList(bytes []byte) ([]corev1.Node, error) { //TODO: this is inefficient, fix later.
	obj, err := runtime.Decode(codec, bytes)
	if err != nil {
		slog.Error("cannot decode as node list", "error", err)
		return nil, err
	}
	var list *corev1.List
	var nodes []corev1.Node
	list = obj.(*corev1.List)
	for _, item := range list.Items {
		var node *corev1.Node
		bytes := item.Raw
		decodedObj, err := runtime.Decode(codec, bytes)
		if err != nil {
			slog.Error("cannot decode node", "error", err)
			return nil, err
		}
		node = decodedObj.(*corev1.Node)
		nodes = append(nodes, *node)
	}
	return nodes, nil
}

func ReadPod(projRelPath string) (corev1.Pod, error) {
	workingDir, _ := os.Getwd() // FIXME: package var not accessible for some reason.
	path := workingDir + "/" + projRelPath
	bytes, err := os.ReadFile(path)
	if err != nil {
		return corev1.Pod{}, err
	}

	pod := corev1.Pod{}
	obj, err := runtime.Decode(codec, bytes)
	if err != nil {
		slog.Error("cannot decode pod object", "error", err)
		return corev1.Pod{}, err
	}
	pod = *(obj.(*corev1.Pod))
	return pod, nil
}
