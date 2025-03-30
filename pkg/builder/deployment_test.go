package builder_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var _ builder.DeploymentBuilder = &TrinoDeploymentBuilder{}

type TrinoDeploymentBuilder struct {
	builder.Deployment
}

func (b *TrinoDeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	fooContainer := builder.NewContainerBuilder("coordinator", b.GetImage()).
		SetCommand([]string{"bin/launcher"}).
		SetArgs([]string{"run"}).
		AddEnvVar(&corev1.EnvVar{
			Name:  "foo",
			Value: "bar",
		}).
		SetResources(b.GetResources()).
		Build()

	b.AddContainer(fooContainer)

	return b.GetObject()
}

var _ = Describe("DeploymentBuilder test", func() {
	ownerName := "fake-owner"
	fakeOwner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ownerName,
			Namespace: "fake-namespace",
			UID:       types.UID("fake-uid"),
		},
	}
	resourceClient := client.NewClient(k8sClient, fakeOwner)
	roleName := "coordinator"
	roleGroupName := "default"

	Context("DeploymentBuilder", func() {

		It("should return a Deployment object", func() {
			By("creating a DeploymentBuilder")
			deploymentBuilder := &TrinoDeploymentBuilder{
				Deployment: *builder.NewDeployment(
					resourceClient,
					"sample-trinocluster-default",
					ptr.To[int32](1),
					util.NewImage("trino", "485", "1.0.0"),
					nil,
					&commonsv1alpha1.RoleGroupConfigSpec{
						Resources: &commonsv1alpha1.ResourcesSpec{
							CPU: &commonsv1alpha1.CPUResource{
								Max: resource.MustParse("100m"),
								Min: resource.MustParse("50m"),
							},
							Memory: &commonsv1alpha1.MemoryResource{
								Limit: resource.MustParse("100Mi"),
							},
						},
					},
					func(o *builder.Options) {
						o.ClusterName = ownerName
						o.RoleName = roleName
						o.RoleGroupName = roleGroupName
						o.Labels = map[string]string{
							constants.LabelKubernetesInstance:  ownerName,
							constants.LabelKubernetesManagedBy: "trino.kubedoop.dev",
							constants.LabelKubernetesComponent: roleName,
							constants.LabelKubernetesName:      "TrinoCluster",
							constants.LabelKubernetesRoleGroup: roleGroupName,
						}

					},
				),
			}

			By("building a Deployment object")
			obj, err := deploymentBuilder.Build(context.Background())
			Expect(err).ToNot(HaveOccurred())

			By("casting the object to a Deployment")
			deployment, ok := obj.(*appv1.Deployment)
			Expect(ok).To(BeTrue())

			By("validating the Deployment object")
			Expect(deployment.Name).To(Equal("sample-trinocluster-default"))
			Expect(deployment.Namespace).To(Equal("fake-namespace"))
			Expect(*deployment.Spec.Replicas).To(BeNumerically("==", 1))

			By("validating the Deployment object's labels")
			labels := deployment.Spec.Template.Labels
			Expect(labels).To(HaveKeyWithValue(constants.LabelKubernetesInstance, ownerName))
			Expect(labels).To(HaveKeyWithValue(constants.LabelKubernetesManagedBy, "trino.kubedoop.dev"))
			Expect(labels).To(HaveKeyWithValue(constants.LabelKubernetesComponent, "coordinator"))
			Expect(labels).To(HaveKeyWithValue(constants.LabelKubernetesRoleGroup, "default"))

			By("validating the Deployment object's match labels")
			matchLabels := deployment.Spec.Selector.MatchLabels
			Expect(matchLabels).To(HaveKeyWithValue(constants.LabelKubernetesInstance, ownerName))
			Expect(matchLabels).To(HaveKeyWithValue(constants.LabelKubernetesManagedBy, "trino.kubedoop.dev"))
			Expect(matchLabels).To(HaveKeyWithValue(constants.LabelKubernetesComponent, "coordinator"))
			Expect(matchLabels).To(HaveKeyWithValue(constants.LabelKubernetesRoleGroup, "default"))

			By("validating the Deployment object's containers")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := deployment.Spec.Template.Spec.Containers[0]

			By("validating the Deployment object's container command")
			Expect(container.Name).To(Equal("coordinator"))

			By("validating the Deployment object's container args")
			Expect(container.Args).To(Equal([]string{"run"}))

			By("validating the Deployment object's container env")
			Expect(container.Env).To(HaveLen(1))
			Expect(func() bool {
				for _, env := range container.Env {
					if env.Name == "foo" && env.Value == "bar" {
						return true
					}
				}
				return false
			}()).To(BeTrue())

			By("validating the Deployment object's resource")
			Expect(container.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("50m")))
			Expect(container.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse("100Mi")))
			Expect(container.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("100m")))
		})

		It("should return a Deployment object with CliOverrides and EnvOverrides", func() {
			By("creating a DeploymentBuilder")
			deploymentBuilder := &TrinoDeploymentBuilder{
				Deployment: *builder.NewDeployment(
					resourceClient,
					"sample-trinocluster-default",
					ptr.To[int32](1),
					util.NewImage("trino", "485", "1.0.0"),
					&commonsv1alpha1.OverridesSpec{
						CliOverrides: []string{
							"bin/launcher",
							"start",
						},
						EnvOverrides: map[string]string{
							"foo": "test",
							"bar": "test",
						},
					},
					nil,
					func(o *builder.Options) {
						o.ClusterName = ownerName
						o.RoleName = roleName
						o.RoleGroupName = roleGroupName
					},
				),
			}

			By("building a Deployment object")
			obj, err := deploymentBuilder.Build(context.Background())
			Expect(err).ToNot(HaveOccurred())

			By("casting the object to a Deployment")
			deployment, ok := obj.(*appv1.Deployment)
			Expect(ok).To(BeTrue())

			By("validating the Deployment object's containers")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := deployment.Spec.Template.Spec.Containers[0]

			By("validating the Deployment object's container command")
			Expect(container.Name).To(Equal("coordinator"))

			By("validating the Deployment object's container command overrides")
			Expect(container.Command).To(Equal([]string{"bin/launcher", "start"}))

			By("validating the Deployment object's container env overrides")
			containerEnv := container.Env
			Expect(containerEnv).ToNot(BeEmpty())
			Expect(containerEnv).To(ContainElement(corev1.EnvVar{
				Name:      "foo",
				Value:     "test",
				ValueFrom: nil,
			}))
			Expect(containerEnv).To(ContainElement(corev1.EnvVar{
				Name:      "bar",
				Value:     "test",
				ValueFrom: nil,
			}))
		})

		It("should return a Deployment object with PodOverrides", func() {
			By("creating a DeploymentBuilder")

			overridesPodTemplate := &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{
						"foo": "bar",
					},
					Containers: []corev1.Container{
						{
							Name:  "foo",
							Image: "bar",
						},
					},
				},
			}

			overridesPodTemplateBytes, err := json.Marshal(overridesPodTemplate)
			Expect(err).ToNot(HaveOccurred())

			deploymentBuilder := &TrinoDeploymentBuilder{
				Deployment: *builder.NewDeployment(
					resourceClient,
					"sample-trinocluster-default",
					ptr.To[int32](1),
					util.NewImage("trino", "485", "1.0.0"),
					&commonsv1alpha1.OverridesSpec{
						EnvOverrides: map[string]string{
							"foo": "test",
						},
						PodOverrides: &runtime.RawExtension{Raw: overridesPodTemplateBytes},
					},
					nil,
					func(o *builder.Options) {
						o.ClusterName = ownerName
						o.RoleName = roleName
						o.RoleGroupName = roleGroupName
					},
				),
			}

			By("building a Deployment object")
			obj, err := deploymentBuilder.Build(context.Background())
			Expect(err).ToNot(HaveOccurred())

			By("casting the object to a Deployment")
			deployment, ok := obj.(*appv1.Deployment)
			Expect(ok).To(BeTrue())

			By("validating the Deployment object's containers")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))

			By("validating the Deployment object's pod overrides")
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(HaveKeyWithValue("foo", "bar"))

			var mainContainer *corev1.Container
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == roleName {
					mainContainer = &container
					break
				}
			}

			Expect(mainContainer).ToNot(BeNil())

			By("validating the Deployment object's container env overrides")
			containerEnv := mainContainer.Env
			Expect(containerEnv).ToNot(BeEmpty())
			Expect(containerEnv).To(ContainElement(corev1.EnvVar{
				Name:      "foo",
				Value:     "test",
				ValueFrom: nil,
			}))
		})
	})
})
