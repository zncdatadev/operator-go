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
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var _ JobBuilder = &Job{}

type Job struct {
	BaseWorkloadBuilder

	resetPolicy *corev1.RestartPolicy
}

func NewGenericJobBuilder(
	client *resourceClient.Client,
	name string, // this is resource name when creating
	image *util.Image,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
	options ...Option,
) JobBuilder {
	return &Job{
		BaseWorkloadBuilder: *NewBaseWorkloadBuilder(
			client,
			name,
			image,
			overrides,
			roleGroupConfig,
			options...,
		),
	}
}

func (b *Job) GetObject() (*batchv1.Job, error) {
	podTemplate, err := b.GetPodTemplate()
	if err != nil {
		return nil, err
	}

	if b.resetPolicy != nil {
		podTemplate.Spec.RestartPolicy = *b.resetPolicy
	}

	obj := &batchv1.Job{
		ObjectMeta: b.GetObjectMeta(),
		Spec: batchv1.JobSpec{
			// Selector: b.GetLabelSelector(),
			Template: *podTemplate,
		},
	}
	return obj, nil
}

func (b *Job) SetRestPolicy(policy *corev1.RestartPolicy) {
	b.resetPolicy = policy
}

func (b *Job) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}
