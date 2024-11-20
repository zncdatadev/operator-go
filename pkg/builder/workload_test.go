package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const testMainContainerName = "main"

var _ = Describe("BaseWorkloadBuilder", func() {
	var cli *client.Client

	BeforeEach(func() {

		cli = client.NewClient(k8sClient, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "test-namespace",
			},
		})
	})

	Describe("AddContainer", func() {
		It("should add and retrieve container correctly", func() {
			builder := builder.NewBaseWorkloadBuilder(cli, "test", &util.Image{}, nil, nil)
			container := &corev1.Container{
				Name:  "test-container",
				Image: "test-image",
			}

			builder.AddContainer(container)
			got := builder.GetContainer("test-container")
			Expect(got).To(Equal(container))
		})
	})

	Describe("SetResources", func() {
		It("should set resources correctly", func() {
			builder := builder.NewBaseWorkloadBuilder(cli, "test", &util.Image{}, nil, nil)

			cpuMin := resource.MustParse("100m")
			cpuMax := resource.MustParse("200m")
			memLimit := resource.MustParse("1Gi")

			resources := &commonsv1alpha1.ResourcesSpec{
				CPU: &commonsv1alpha1.CPUResource{
					Min: cpuMin,
					Max: cpuMax,
				},
				Memory: &commonsv1alpha1.MemoryResource{
					Limit: memLimit,
				},
			}

			builder.SetResources(resources)
			builder.AddContainer(&corev1.Container{Name: builder.RoleName})

			podTemplate, err := builder.GetPodTemplate()
			Expect(err).NotTo(HaveOccurred())
			container := podTemplate.Spec.Containers[0]

			Expect(container).NotTo(BeNil())
			Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(cpuMin))
			Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(cpuMax))
			Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(memLimit))
		})
	})

	Describe("Replicas", func() {
		It("should handle replicas correctly", func() {
			replicas := int32(3)
			builder := builder.NewBaseWorkloadReplicasBuilder(cli, "test", &replicas, &util.Image{}, nil, nil)

			Expect(builder.GetReplicas()).To(Equal(&replicas))

			builder.SetReplicas(nil)
			defaultReplicas := int32(1)
			Expect(builder.GetReplicas()).To(Equal(&defaultReplicas))
		})
	})

	Describe("SecurityContext", func() {
		It("should set security context correctly", func() {
			builder := builder.NewBaseWorkloadBuilder(cli, "test", &util.Image{}, nil, nil)

			user := int64(1000)
			group := int64(1000)
			nonRoot := true

			builder.SetSecurityContext(user, group, nonRoot)

			sc := builder.GetSecurityContext()
			Expect(sc).NotTo(BeNil())
			Expect(sc.RunAsUser).To(Equal(&user))
			Expect(sc.RunAsGroup).To(Equal(&group))
			Expect(sc.RunAsNonRoot).To(Equal(&nonRoot))
		})
	})

	Describe("Overrides", func() {
		Context("CLI Overrides", func() {
			It("should apply CLI overrides correctly", func() {
				overrides := &commonsv1alpha1.OverridesSpec{
					CliOverrides: []string{"/custom/command", "--flag1", "--flag2"},
				}

				builder := builder.NewBaseWorkloadBuilder(cli, "test", &util.Image{}, overrides, nil)
				builder.RoleName = testMainContainerName
				container := &corev1.Container{
					Name:    testMainContainerName,
					Command: []string{"/original/command"},
					Args:    []string{"--original-flag"},
				}
				builder.AddContainer(container)

				builder.OverrideContainer()

				updatedContainer := builder.GetContainer(testMainContainerName)
				Expect(updatedContainer).NotTo(BeNil())
				Expect(updatedContainer.Command).To(Equal(overrides.CliOverrides))
				Expect(updatedContainer.Args).To(BeEmpty())
			})
		})

		Context("Environment Overrides", func() {
			It("should apply environment overrides correctly", func() {
				overrides := &commonsv1alpha1.OverridesSpec{
					EnvOverrides: map[string]string{
						"TEST_ENV": "test-value",
						"DEBUG":    "true",
					},
				}

				builder := builder.NewBaseWorkloadBuilder(cli, "test", &util.Image{}, overrides, nil)
				builder.RoleName = testMainContainerName
				container := &corev1.Container{
					Name: testMainContainerName,
					Env: []corev1.EnvVar{
						{Name: "EXISTING_ENV", Value: "existing-value"},
					},
				}
				builder.AddContainer(container)

				builder.OverrideContainer()

				updatedContainer := builder.GetContainer(testMainContainerName)
				Expect(updatedContainer).NotTo(BeNil())
				envMap := getEnvMap(updatedContainer.Env)
				Expect(envMap).To(HaveKey("EXISTING_ENV"))
				Expect(envMap["TEST_ENV"]).To(Equal("test-value"))
				Expect(envMap["DEBUG"]).To(Equal("true"))
			})
		})

		Context("Pod Template Overrides", func() {
			It("should apply pod template overrides correctly", func() {
				rawPodOverrides := runtime.RawExtension{
					Raw: []byte(`{
						"spec": {
							"hostNetwork": true,
							"dnsPolicy": "ClusterFirstWithHostNet",
							"containers": [{
								"name": "main",
								"resources": {
									"limits": {
										"memory": "2Gi"
									}
								}
							}]
						}
					}`),
				}

				overrides := &commonsv1alpha1.OverridesSpec{
					PodOverrides: &rawPodOverrides,
				}

				builder := builder.NewBaseWorkloadBuilder(cli, "test", &util.Image{}, overrides, nil)
				builder.RoleName = testMainContainerName
				builder.AddContainer(&corev1.Container{Name: testMainContainerName})

				podTemplate, err := builder.GetPodTemplate()
				Expect(err).NotTo(HaveOccurred())
				Expect(podTemplate).NotTo(BeNil())
				Expect(podTemplate.Spec.HostNetwork).To(BeTrue())
				Expect(podTemplate.Spec.DNSPolicy).To(Equal(corev1.DNSClusterFirstWithHostNet))

				mainContainer := findContainer(podTemplate.Spec.Containers, testMainContainerName)
				Expect(mainContainer).NotTo(BeNil())
				Expect(mainContainer.Resources.Limits.Memory().String()).To(Equal("2Gi"))
			})
		})
	})
})

// Helper functions
func getEnvMap(envVars []corev1.EnvVar) map[string]string {
	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}
	return envMap
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for _, c := range containers {
		if c.Name == name {
			return &c
		}
	}
	return nil
}
