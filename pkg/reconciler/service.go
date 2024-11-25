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

package reconciler

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
)

var _ ResourceReconciler[builder.ServiceBuilder] = &Service{}

type Service struct {
	GenericResourceReconciler[builder.ServiceBuilder]
}

func NewServiceReconciler(
	client *client.Client,
	name string,
	ports []corev1.ContainerPort,
	options ...builder.ServiceBuilderOption,
) *Service {
	svcBuilder := builder.NewServiceBuilder(
		client,
		name,
		ports,
		options...,
	)
	return &Service{
		GenericResourceReconciler: *NewGenericResourceReconciler[builder.ServiceBuilder](
			client,
			svcBuilder,
		),
	}
}
