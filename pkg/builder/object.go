/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builder

import (
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	logger = ctrl.Log.WithName("builder")
)

var _ ObjectMetaBuilder = &ObjectMeta{}

type ObjectMeta struct {
	Client *client.Client

	Name        string // this is resource name when creating
	labels      map[string]string
	annotations map[string]string

	ClusterName   string
	RoleName      string
	RoleGroupName string
}

func NewObjectMeta(
	client *client.Client,
	name string, // this is resource name when creating
	options ...Option,
) *ObjectMeta {

	opts := &Options{}
	for _, opt := range options {
		opt(opts)
	}

	return &ObjectMeta{
		Client:        client,
		Name:          name,
		labels:        opts.Labels,
		annotations:   opts.Annotations,
		ClusterName:   opts.ClusterName,
		RoleName:      opts.RoleName,
		RoleGroupName: opts.RoleGroupName,
	}
}

func (b *ObjectMeta) GetClient() *client.Client {
	return b.Client
}

func (b *ObjectMeta) SetName(name string) {
	b.Name = name
}

func (b *ObjectMeta) GetName() string {
	return b.Name
}

func (b *ObjectMeta) AddLabels(labels map[string]string) {
	if b.labels == nil {
		b.labels = make(map[string]string)
	}
	for k, v := range labels {
		b.labels[k] = v
	}
}

func (b *ObjectMeta) GetLabels() map[string]string {
	if b.labels == nil {
		b.labels = map[string]string{
			constants.LabelKubernetesInstance:  b.Client.GetOwnerName(),
			constants.LabelKubernetesManagedBy: constants.KubedoopDomain,
		}

		if b.ClusterName != "" {
			b.labels[constants.LabelKubernetesInstance] = b.ClusterName
		}

		if b.RoleName != "" {
			b.labels[constants.LabelKubernetesComponent] = b.RoleName
		}

		if b.RoleGroupName != "" {
			b.labels[constants.LabelKubernetesRoleGroup] = b.RoleGroupName
		}
	}

	return b.labels
}

func (o *ObjectMeta) filterLabels(labels map[string]string) map[string]string {
	matchingLabels := make(map[string]string)
	for _, label := range constants.MatchingLabelsNames() {
		if value, ok := labels[label]; ok {
			matchingLabels[label] = value
		}
	}
	return matchingLabels
}

func (b *ObjectMeta) GetMatchingLabels() map[string]string {
	return b.filterLabels(b.GetLabels())
}

func (b *ObjectMeta) GetLabelSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: b.GetMatchingLabels(),
	}
}

func (b *ObjectMeta) AddAnnotations(annotations map[string]string) {
	if b.annotations == nil {
		b.annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.annotations[k] = v
	}
}

func (b *ObjectMeta) GetAnnotations() map[string]string {
	return b.annotations
}

func (b *ObjectMeta) GetObjectMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        b.GetName(),
		Namespace:   b.Client.GetOwnerNamespace(),
		Labels:      b.GetLabels(),
		Annotations: b.annotations,
	}
}

// GetObjectMetaWithClusterScope returns the object meta with cluster scope,
// meaning that the object is not namespaced.
func (b *ObjectMeta) GetObjectMetaWithClusterScope() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        b.GetName(),
		Labels:      b.GetLabels(),
		Annotations: b.annotations,
	}
}
