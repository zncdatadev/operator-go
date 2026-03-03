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

package handlers

import (
	"fmt"

	trinov1alpha1 "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1"
	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/constants"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// BuildHeadlessService builds a headless service for StatefulSet using SDK builder
func BuildHeadlessService(buildCtx *reconciler.RoleGroupBuildContext, port int32) *corev1.Service {
	return builder.NewHeadlessServiceBuilder(
		fmt.Sprintf("%s-headless", buildCtx.ResourceName),
		buildCtx.ClusterNamespace,
	).
		WithLabels(buildCtx.ClusterLabels).
		WithSelector(buildCtx.ClusterLabels).
		AddPortSimple("http", port, corev1.ProtocolTCP).
		Build()
}

// BuildService builds a client-facing service using SDK builder
func BuildService(buildCtx *reconciler.RoleGroupBuildContext, port int32) *corev1.Service {
	return builder.NewServiceBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).
		WithLabels(buildCtx.ClusterLabels).
		WithSelector(buildCtx.ClusterLabels).
		AddPortSimple("http", port, corev1.ProtocolTCP).
		WithServiceType(builder.ServiceTypeClusterIP).
		Build()
}

// BuildConfigMap builds a ConfigMap with Trino configuration using SDK builder
func BuildConfigMap(buildCtx *reconciler.RoleGroupBuildContext, data map[string]string) *corev1.ConfigMap {
	cmBuilder := builder.NewConfigMapBuilder(
		fmt.Sprintf("%s-config", buildCtx.ResourceName),
		buildCtx.ClusterNamespace,
	).WithLabels(buildCtx.ClusterLabels)

	for key, value := range data {
		cmBuilder.AddData(key, value)
	}

	return cmBuilder.Build()
}

// BuildStatefulSet builds a StatefulSet using SDK builder
func BuildStatefulSet(
	buildCtx *reconciler.RoleGroupBuildContext,
	cr *trinov1alpha1.TrinoCluster,
	configMapName string,
	port int32,
	replicas int32,
	cpuRequest, cpuLimit, memoryLimit string,
) *appsv1.StatefulSet {
	// Build resources spec
	maxCPU := resource.MustParse(cpuLimit)
	minCPU := resource.MustParse(cpuRequest)
	memLimit := resource.MustParse(memoryLimit)
	resources := &v1alpha1.ResourcesSpec{
		CPU:    &v1alpha1.CPUResource{Max: maxCPU, Min: minCPU},
		Memory: &v1alpha1.MemoryResource{Limit: memLimit},
	}

	stsBuilder := builder.NewStatefulSetBuilder(buildCtx.ResourceName, buildCtx.ClusterNamespace).
		WithLabels(buildCtx.ClusterLabels).
		WithReplicas(replicas).
		WithImage(cr.Spec.Image, corev1.PullIfNotPresent).
		WithResources(resources).
		AddPort("http", port, corev1.ProtocolTCP).
		AddVolume(corev1.Volume{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		}).
		AddVolumeMount(corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/trino",
		})

	return stsBuilder.Build()
}

// BuildPDB builds a PodDisruptionBudget using SDK builder
func BuildPDB(
	buildCtx *reconciler.RoleGroupBuildContext,
	pdbSpec *v1alpha1.PodDisruptionBudgetSpec,
) *policyv1.PodDisruptionBudget {
	if pdbSpec == nil || !pdbSpec.Enabled {
		return nil
	}

	pdbBuilder := builder.NewPDBBuilder(
		fmt.Sprintf("%s-pdb", buildCtx.ResourceName),
		buildCtx.ClusterNamespace,
	).
		WithLabels(buildCtx.ClusterLabels).
		WithSelector(buildCtx.ClusterLabels).
		WithSpec(pdbSpec)

	return pdbBuilder.Build()
}

// GetCoordinatorPort returns the coordinator HTTP port from spec or default
func GetCoordinatorPort(cr *trinov1alpha1.TrinoCluster) int32 {
	if cr.Spec.Coordinators != nil && cr.Spec.Coordinators.HTTPPort != 0 {
		return cr.Spec.Coordinators.HTTPPort
	}
	return constants.DefaultHTTPPort
}

// GetWorkerPort returns the worker HTTP port from spec or default
func GetWorkerPort(cr *trinov1alpha1.TrinoCluster) int32 {
	if cr.Spec.Workers != nil && cr.Spec.Workers.HTTPPort != 0 {
		return cr.Spec.Workers.HTTPPort
	}
	return constants.DefaultHTTPPort
}

// GetDiscoveryURI returns the discovery URI for the coordinator
func GetDiscoveryURI(cr *trinov1alpha1.TrinoCluster, port int32) string {
	return fmt.Sprintf("http://%s-coordinator:%d", cr.Name, port)
}

// GetReplicas returns the replicas from RoleGroupSpec or default value
func GetReplicas(buildCtx *reconciler.RoleGroupBuildContext, defaultReplicas int32) int32 {
	if buildCtx.RoleGroupSpec.Replicas != nil {
		return *buildCtx.RoleGroupSpec.Replicas
	}
	return defaultReplicas
}

// ConvertStringToIntOrString converts a string to intstr.IntOrString
func ConvertStringToIntOrString(s string) intstr.IntOrString {
	return intstr.Parse(s)
}
