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

package testutil

import (
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewTestConfigMap creates a test ConfigMap.
func NewTestConfigMap(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}
}

// NewTestConfigMapWithData creates a test ConfigMap with custom data.
func NewTestConfigMapWithData(name, namespace string, data map[string]string) *corev1.ConfigMap {
	cm := NewTestConfigMap(name, namespace)
	cm.Data = data
	return cm
}

// NewTestService creates a test Service.
func NewTestService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
	}
}

// NewTestHeadlessService creates a test headless Service.
func NewTestHeadlessService(name, namespace string) *corev1.Service {
	svc := NewTestService(name+"-headless", namespace)
	svc.Spec.ClusterIP = "None"
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	return svc
}

// NewTestServiceWithPorts creates a test Service with custom ports.
func NewTestServiceWithPorts(name, namespace string, ports []corev1.ServicePort) *corev1.Service {
	svc := NewTestService(name, namespace)
	svc.Spec.Ports = ports
	return svc
}

// NewTestStatefulSet creates a test StatefulSet.
func NewTestStatefulSet(name, namespace string) *appsv1.StatefulSet {
	replicas := int32(1)
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name + "-headless",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: "test-image:latest",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}
}

// NewTestStatefulSetBuilder creates a StatefulSetBuilder for testing.
func NewTestStatefulSetBuilder(name, namespace string) *builder.StatefulSetBuilder {
	return builder.NewStatefulSetBuilder(name, namespace).
		WithLabels(map[string]string{
			"app.kubernetes.io/name": name,
		}).
		WithReplicas(1)
}

// NewTestServiceBuilder creates a ServiceBuilder for testing.
func NewTestServiceBuilder(name, namespace string) *builder.ServiceBuilder {
	return builder.NewServiceBuilder(name, namespace).
		WithLabels(map[string]string{
			"app.kubernetes.io/name": name,
		})
}

// NewTestConfigMapBuilder creates a ConfigMapBuilder for testing.
func NewTestConfigMapBuilder(name, namespace string) *builder.ConfigMapBuilder {
	return builder.NewConfigMapBuilder(name, namespace).
		WithLabels(map[string]string{
			"app.kubernetes.io/name": name,
		})
}

// NewTestPDB creates a test PodDisruptionBudget.
func NewTestPDB(name, namespace string) *policyv1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": name,
				},
			},
		},
	}
}

// NewTestPDBWithMaxUnavailable creates a PDB with maxUnavailable.
func NewTestPDBWithMaxUnavailable(name, namespace string, maxUnavailable int32) *policyv1.PodDisruptionBudget {
	pdb := NewTestPDB(name, namespace)
	max := intstr.FromInt(int(maxUnavailable))
	pdb.Spec.MaxUnavailable = &max
	return pdb
}

// NewTestMergedConfig creates a test MergedConfig.
func NewTestMergedConfig() *config.MergedConfig {
	return &config.MergedConfig{
		EnvVars: map[string]string{
			"TEST_VAR": "test-value",
		},
		CliArgs: []string{"--test-arg=test-value"},
	}
}

// NewTestMergedConfigWithEnv creates a MergedConfig with environment variables.
func NewTestMergedConfigWithEnv(envVars map[string]string) *config.MergedConfig {
	cfg := NewTestMergedConfig()
	for k, v := range envVars {
		cfg.EnvVars[k] = v
	}
	return cfg
}

// NewTestRoleGroupSpec creates a test RoleGroupSpec.
func NewTestRoleGroupSpec(replicas int32) *v1alpha1.RoleGroupSpec {
	return &v1alpha1.RoleGroupSpec{
		Replicas: &replicas,
	}
}

// Helper functions

// PtrInt32 returns a pointer to an int32.
func PtrInt32(i int32) *int32 {
	return &i
}

// PtrInt64 returns a pointer to an int64.
func PtrInt64(i int64) *int64 {
	return &i
}

// NewTestNamespace creates a test Namespace.
func NewTestNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "test",
			},
		},
	}
}

// NewTestPod creates a test Pod.
func NewTestPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  name,
					Image: "test-image:latest",
				},
			},
		},
	}
}

// NewTestSecret creates a test Secret.
func NewTestSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte("test-user"),
			"password": []byte("test-password"),
		},
	}
}
