package simutil

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
)

const (
	defaultNamespace      = "default"
	defaultContainerImage = "registry.k8s.io/pause:3.5"
	defaultContainerName  = "pause"
	defaultSchedulerName  = "default-scheduler"
)

type PodBuilder struct {
	objectMeta      metav1.ObjectMeta
	schedulerName   string
	nodeName        string
	resourceRequest corev1.ResourceList
	tsc             []corev1.TopologySpreadConstraint
}

func NewPodBuilder() *PodBuilder {
	return &PodBuilder{
		resourceRequest: make(corev1.ResourceList),
		objectMeta: metav1.ObjectMeta{
			Namespace: defaultNamespace,
			Labels:    make(map[string]string),
		},
	}
}

func (p *PodBuilder) Name(name string) *PodBuilder {
	p.objectMeta.Name = name
	p.objectMeta.GenerateName = ""
	return p
}

func (p *PodBuilder) GenerateName(generateName string) *PodBuilder {
	p.objectMeta.GenerateName = generateName
	p.objectMeta.Name = ""
	return p
}

func (p *PodBuilder) Namespace(namespace string) *PodBuilder {
	p.objectMeta.Namespace = namespace
	return p
}

func (p *PodBuilder) AddLabels(labels map[string]string) *PodBuilder {
	p.objectMeta.Labels = labels
	return p
}

func (p *PodBuilder) SchedulerName(schedulerName string) *PodBuilder {
	p.schedulerName = schedulerName
	return p
}

func (p *PodBuilder) RequestMemory(quantity string) *PodBuilder {
	p.resourceRequest[corev1.ResourceMemory] = resource.MustParse(quantity)
	return p
}

func (p *PodBuilder) RequestCPU(quantity string) *PodBuilder {
	p.resourceRequest[corev1.ResourceCPU] = resource.MustParse(quantity)
	return p
}

func (p *PodBuilder) NodeName(nodeName string) *PodBuilder {
	p.nodeName = nodeName
	return p
}

func (p *PodBuilder) TopologySpreadConstraint(topologyKey *string, maxSkew *int, labels map[string]string) *PodBuilder {
	if topologyKey == nil || maxSkew == nil || labels == nil {
		return p
	}
	p.tsc = []corev1.TopologySpreadConstraint{
		{
			TopologyKey:       *topologyKey,
			WhenUnsatisfiable: corev1.DoNotSchedule,
			MaxSkew:           int32(*maxSkew),
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			MinDomains: pointer.Int32(3),
		},
	}
	return p
}

func (p *PodBuilder) Build() (*corev1.Pod, error) {
	if p.resourceRequest == nil || len(p.resourceRequest) == 0 {
		return nil, fmt.Errorf("resource request must be set")
	}
	return &corev1.Pod{
		ObjectMeta: p.objectMeta,
		Spec: corev1.PodSpec{
			SchedulerName: EmptyOr(p.schedulerName, defaultSchedulerName),
			Containers: []corev1.Container{
				{
					Name:  defaultContainerName,
					Image: defaultContainerImage,
					Resources: corev1.ResourceRequirements{
						Requests: p.resourceRequest,
					},
				},
			},
			NodeName:                      p.nodeName,
			TerminationGracePeriodSeconds: ptr.To(int64(0)),
			TopologySpreadConstraints:     p.tsc,
		},
	}, nil
}
