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

	appv1 "k8s.io/api/apps/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	client "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var (
	DefaultReplicas = int32(1)
)

var _ DeploymentBuilder = &Deployment{}

type Deployment struct {
	BaseWorkloadReplicasBuilder
}

func NewDeployment(
	client *client.Client,
	name string,
	replicas *int32,
	image *util.Image,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
	options ...Option,
) *Deployment {
	return &Deployment{
		BaseWorkloadReplicasBuilder: *NewBaseWorkloadReplicasBuilder(
			client,
			name,
			replicas,
			image,
			overrides,
			roleGroupConfig,
			options...,
		),
	}
}

func (b *Deployment) GetObject() (*appv1.Deployment, error) {
	tpl, err := b.GetPodTemplate()
	if err != nil {
		return nil, err
	}

	obj := &appv1.Deployment{
		ObjectMeta: b.GetObjectMeta(),
		Spec: appv1.DeploymentSpec{
			Replicas: b.GetReplicas(),
			Selector: b.GetLabelSelector(),
			Template: *tpl,
		},
	}

	return obj, nil
}

func (b *Deployment) Build(_ context.Context) (ctrlclient.Object, error) {
	return b.GetObject()
}
