package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type RoleReconciler struct {
	reconciler.BaseRoleReconciler[GiteaSpec]
	ClusterConfig *ClusterConfigSpec // add more fields in implementation
}

func NewRoleReconciler(
	client *client.Client,
	roleInfo *reconciler.RoleInfo,
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	clusterConfig *ClusterConfigSpec,
	spec GiteaSpec,
) *RoleReconciler {
	return &RoleReconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler[GiteaSpec](
			client,
			roleInfo,
			clusterOperation,
			spec,
		),
		ClusterConfig: clusterConfig,
	}
}

// RegisterResourcesWithReflect registers resources with reflect
func (r *RoleReconciler) RegisterResourcesWithReflect(ctx context.Context) error {
	roleGroup, err := r.GetRoleGroups()
	if err != nil {
		return err
	}

	for roleGroupName, roleGroupSpec := range roleGroup {
		info := &reconciler.RoleGroupInfo{
			RoleInfo:  *r.RoleInfo,
			GroupName: roleGroupName,
		}

		reconcilers, err := r.RegisterResourceWithRoleGroup(ctx, info, roleGroupSpec)
		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
		}
	}
	return nil
}

// RegisterResources registers resources with T
func (r *RoleReconciler) RegisterResources(ctx context.Context) error {
	for roleGroupName, roleGroupSpec := range r.Spec.RoleGroups {
		info := &reconciler.RoleGroupInfo{
			RoleInfo:  *r.RoleInfo,
			GroupName: roleGroupName,
		}

		reconcilers, err := r.RegisterResourceWithRoleGroup(ctx, info, &roleGroupSpec)
		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
		}
	}
	return nil
}

func (r *RoleReconciler) RegisterResourceWithRoleGroup(ctx context.Context, info *reconciler.RoleGroupInfo, roleGroupSpec any) ([]reconciler.Reconciler, error) {

	// roleGroupSpec convert to GiteaRoleGroupSpec
	roleGroup := roleGroupSpec.(*GiteaRoleGroupSpec)

	reconcilers := []reconciler.Reconciler{}

	reconcilers = append(reconcilers, r.getServiceReconciler(info))

	reconcilers = append(reconcilers, r.getDeployment(info, roleGroup))

	return reconcilers, nil
}

func (r *RoleReconciler) getDeployment(info *reconciler.RoleGroupInfo, roleGroup *GiteaRoleGroupSpec) reconciler.Reconciler {

	// Create a deployment builder
	deploymentBuilder := &FooDeploymentBuilder{
		Deployment: *builder.NewDeployment(
			r.Client,
			info.GetFullName(),
			info.GetLabels(),
			info.GetAnnotations(),
			nil,
			nil,
			nil,
			roleGroup.Replicas,
		),
	}
	// Create a deployment reconciler
	return reconciler.NewDeployment(r.Client, info.GetFullName(), deploymentBuilder)
}

func (r *RoleReconciler) getServiceReconciler(info *reconciler.RoleGroupInfo) reconciler.Reconciler {
	return reconciler.NewServiceReconciler(
		r.Client,
		info.GetFullName(),
		info.GetLabels(),
		info.GetAnnotations(),
		[]corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 3000,
			},
		},
	)
}

var _ = Describe("Role reconciler", func() {

	Context("RoleReconciler test", func() {
		var resourceClient *client.Client

		roleInfo := &reconciler.RoleInfo{
			ClusterInfo: reconciler.ClusterInfo{
				GVK: &metav1.GroupVersionKind{
					Group:   "fake.zncdata.dev",
					Version: "v1alpha1",
					Kind:    "GiteaCluster",
				},
				ClusterName: "fake-owner",
			},
			RoleName: "gitea",
		}

		var namespace string
		ctx := context.Background()

		giteaRole := GiteaSpec{
			RoleGroups: map[string]GiteaRoleGroupSpec{
				"default": {
					Replicas: &[]int32{1}[0],
				},
			},
		}

		BeforeEach(func() {
			namespace = "test-" + strconv.Itoa(rand.Intn(10000))
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			fakeOwner := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleInfo.GetClusterName(),
					Namespace: namespace,
					UID:       types.UID("fake-uid"),
				},
			}

			resourceClient = client.NewClient(k8sClient, fakeOwner)
		})

		AfterEach(func() {

		})

		It("should reconcile role resource", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&apiv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				giteaRole,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("registering resources")
			Expect(roleReconciler.RegisterResources(ctx)).To(Succeed())

			By("reconciling resources")
			Eventually(func() bool {
				result := roleReconciler.Reconcile(ctx)
				return result.RequeueOrNot()
			}, time.Second*3, time.Second*1).Should(BeFalse())

			By("mock deployment is ready")
			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleInfo.GetFullName() + "-default"}, deployment)).Should(Succeed())
			deployment.Status.Replicas = 1
			deployment.Status.ReadyReplicas = 1
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())

			By("check resource ready")
			Eventually(func() bool {
				result := roleReconciler.Ready(ctx)
				return result.RequeueOrNot()
			}, time.Second*3, time.Second*1).Should(BeFalse())
		})
	})

	Context("RoleReconciler role and roleGroup merge", func() {

		roleInfo := &reconciler.RoleInfo{
			ClusterInfo: reconciler.ClusterInfo{
				GVK: &metav1.GroupVersionKind{
					Group:   "fake.zncdata.dev",
					Version: "v1alpha1",
					Kind:    "GiteaCluster",
				},
				ClusterName: "fake-owner",
			},
			RoleName: "gitea",
		}

		resourceClient := client.NewClient(k8sClient, nil)
		var role *GiteaSpec

		BeforeEach(func() {
			role = &GiteaSpec{
				EnvOverrides: []corev1.EnvVar{
					{
						Name:  "TEST",
						Value: "test",
					},
				},
				Config: &GiteaConfigSpec{
					GracefulShutdownTimeout: &[]string{"10s"}[0],
				},
				CommandOverrides: []string{
					"tail",
				},
				RoleGroups: map[string]GiteaRoleGroupSpec{
					"default": {
						Replicas: &[]int32{1}[0],
						CommandOverrides: []string{
							"echo",
						},
					},
					"test": {
						Replicas: &[]int32{2}[0],
						Config: &GiteaConfigSpec{
							GracefulShutdownTimeout: &[]string{"20s"}[0],
						},
					},
				},
			}
		})

		It("should merged role to roleGroup spec", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&apiv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				*role,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("get role groups")
			roleGroups, err := roleReconciler.GetRoleGroups()
			Expect(err).To(BeNil())
			Expect(roleGroups).ToNot(BeNil())

			By("check default role group")
			defaultRoleGroupValue, ok := roleGroups["default"]
			Expect(ok).To(BeTrue())
			Expect(defaultRoleGroupValue).ToNot(BeNil())
			defaultRoleGroup, ok := defaultRoleGroupValue.(*GiteaRoleGroupSpec)
			Expect(ok).To(BeTrue())
			Expect(defaultRoleGroup).ToNot(BeNil())
			Expect(defaultRoleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
			Expect(defaultRoleGroup.Config).To(Equal(role.Config))
			// defaultRoleGroup.CommandOverrides != role.CommandOverrides
			Expect(defaultRoleGroup.CommandOverrides).ToNot(Equal(role.CommandOverrides))

			By("check test role group")
			testRoleGroupValue, ok := roleGroups["test"]
			Expect(ok).To(BeTrue())
			Expect(testRoleGroupValue).ToNot(BeNil())
			testRoleGroup, ok := testRoleGroupValue.(*GiteaRoleGroupSpec)
			Expect(ok).To(BeTrue())
			Expect(testRoleGroup).ToNot(BeNil())
			Expect(testRoleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
			// testRoleGroup.Config != role.Config
			Expect(testRoleGroup.Config).ToNot(Equal(role.Config))
			Expect(testRoleGroup.CommandOverrides).To(Equal(role.CommandOverrides))

		})
	})

	Context("RoleReconciler merge roleGroup spec", func() {

		roleInfo := &reconciler.RoleInfo{
			ClusterInfo: reconciler.ClusterInfo{
				GVK: &metav1.GroupVersionKind{
					Group:   "fake.zncdata.dev",
					Version: "v1alpha1",
					Kind:    "GiteaCluster",
				},
				ClusterName: "fake-owner",
			},
			RoleName: "gitea",
		}

		resourceClient := client.NewClient(k8sClient, nil)
		var role *GiteaSpec
		var roleGroupOne *GiteaRoleGroupSpec
		var roleGroupTwo *GiteaRoleGroupSpec

		BeforeEach(func() {
			role = &GiteaSpec{
				EnvOverrides: []corev1.EnvVar{
					{
						Name:  "TEST",
						Value: "test",
					},
				},
				Config: &GiteaConfigSpec{
					GracefulShutdownTimeout: &[]string{"10s"}[0],
				},
				CommandOverrides: []string{
					"tail",
				},
			}

			roleGroupOne = &GiteaRoleGroupSpec{
				Replicas: &[]int32{1}[0],
				CommandOverrides: []string{
					"echo",
				},
			}

			roleGroupTwo = &GiteaRoleGroupSpec{
				Replicas: &[]int32{2}[0],
				Config: &GiteaConfigSpec{
					GracefulShutdownTimeout: &[]string{"20s"}[0],
				},
			}
		})

		It("should merged role to roleGroup spec with roleGroupOne", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&apiv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				*role,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("merge role group spec")

			roleGroupValue := roleReconciler.MergeRoleGroupSpec(roleGroupOne)
			Expect(roleGroupValue).ToNot(BeNil())

			By("assert roleGroupValue is GiteaRoleGroupSpec")
			roleGroup, ok := roleGroupValue.(*GiteaRoleGroupSpec)
			Expect(ok).To(BeTrue())

			By("checking role.Config merged")
			Expect(roleGroup.Config.GracefulShutdownTimeout).To(Equal(role.Config.GracefulShutdownTimeout))

			By("checking role.CommandOverrides not merged")
			Expect(roleGroup.CommandOverrides).ToNot(Equal(role.CommandOverrides))

			By("checking role.EnvOverrides merged")
			Expect(roleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
		})

		It("should merged role to roleGroup spec with roleGroupTwo", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&apiv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				*role,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("merge role group spec")

			roleGroupValue := roleReconciler.MergeRoleGroupSpec(roleGroupTwo)
			Expect(roleGroupValue).ToNot(BeNil())

			By("assert roleGroupValue is GiteaRoleGroupSpec")
			roleGroup, ok := roleGroupValue.(*GiteaRoleGroupSpec)
			Expect(ok).To(BeTrue())

			By("checking role.CommandOverrides merged")
			Expect(roleGroup.CommandOverrides).To(Equal(role.CommandOverrides))

			By("checking role.Config not merged")
			Expect(roleGroup.Config.GracefulShutdownTimeout).ToNot(Equal(role.Config.GracefulShutdownTimeout))

			By("checking role.EnvOverrides merged")
			Expect(roleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
		})
	})
})
