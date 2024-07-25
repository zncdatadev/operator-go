package reconciler_test

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
)

var (
	roleLogger = ctrl.Log.WithName("role-reconciler")
)

type RoleReconciler struct {
	reconciler.BaseRoleReconciler[CoordinatorSpec]
	ClusterConfig *ClusterConfigSpec // add more fields in implementation
}

func NewRoleReconciler(
	client *client.Client,
	roleInfo reconciler.RoleInfo,
	clusterOperation *commonsv1alpha1.ClusterOperationSpec,
	clusterConfig *ClusterConfigSpec,
	spec CoordinatorSpec,
) *RoleReconciler {
	return &RoleReconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler[CoordinatorSpec](
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
		info := reconciler.RoleGroupInfo{
			RoleInfo:      r.RoleInfo,
			RoleGroupName: roleGroupName,
		}

		reconcilers, err := r.getResourceWithRoleGroup(ctx, info, roleGroupSpec)
		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
			roleLogger.Info("register resource", "role", r.GetName(), "roleGroup", roleGroupName, "reconciler", reconciler.GetName())
		}
	}
	return nil
}

// RegisterResources registers resources with T
func (r *RoleReconciler) RegisterResources(ctx context.Context) error {
	for roleGroupName, roleGroupSpec := range r.Spec.RoleGroups {

		mergedRoleGroup := r.MergeRoleGroupSpec(&roleGroupSpec)

		info := reconciler.RoleGroupInfo{
			RoleInfo:      r.RoleInfo,
			RoleGroupName: roleGroupName,
		}

		reconcilers, err := r.getResourceWithRoleGroup(ctx, info, mergedRoleGroup)
		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
			roleLogger.Info("register resource", "role", r.GetName(), "roleGroup", roleGroupName, "reconciler", reconciler.GetName())
		}
	}
	return nil
}

func (r *RoleReconciler) getResourceWithRoleGroup(_ context.Context, info reconciler.RoleGroupInfo, roleGroupSpec any) ([]reconciler.Reconciler, error) {

	// roleGroupSpec convert to TrinoRoleGroupSpec
	roleGroup := roleGroupSpec.(*TrinoRoleGroupSpec)

	reconcilers := []reconciler.Reconciler{}

	reconcilers = append(reconcilers, r.getServiceReconciler(info))

	deploymentReconciler, err := r.getDeployment(info, roleGroup)
	if err != nil {
		return nil, err
	}

	reconcilers = append(reconcilers, deploymentReconciler)

	return reconcilers, nil
}

func (r *RoleReconciler) getDeployment(info reconciler.RoleGroupInfo, roleGroup *TrinoRoleGroupSpec) (reconciler.Reconciler, error) {

	options := &builder.WorkloadOptions{
		Labels:           info.GetLabels(),
		Annotations:      info.GetAnnotations(),
		EnvOverrides:     roleGroup.EnvOverrides,
		CommandOverrides: roleGroup.CommandOverrides,
		RoleGroupInfo: &builder.RoleGroupInfo{
			RoleName:      info.RoleName,
			RoleGroupName: info.RoleGroupName,
		},
	}

	if roleGroup.Config != nil {

		var gracefulShutdownTimeout time.Duration
		var err error

		if roleGroup.Config.GracefulShutdownTimeout != "" {
			gracefulShutdownTimeout, err = time.ParseDuration(roleGroup.Config.GracefulShutdownTimeout)

			if err != nil {
				return nil, errors.New("failed to parse graceful shutdown")
			}
		}

		options.TerminationGracePeriod = &gracefulShutdownTimeout

		options.Affinity = roleGroup.Config.Affinity
	}

	// Create a deployment builder
	deploymentBuilder := &TrinoCoordinatorDeploymentBuilder{
		Deployment: *builder.NewDeployment(
			r.GetClient(),
			info.GetFullName(),
			roleGroup.Replicas,
			&util.Image{
				StackVersion:   "1.0.0",
				ProductName:    "trino",
				ProductVersion: "458",
			},
			options,
		),
	}
	// Create a deployment reconciler
	return reconciler.NewDeployment(r.Client, info.GetFullName(), deploymentBuilder), nil
}

func (r *RoleReconciler) getServiceReconciler(info reconciler.RoleGroupInfo) reconciler.Reconciler {
	return reconciler.NewServiceReconciler(
		r.GetClient(),
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

		roleInfo := reconciler.RoleInfo{
			ClusterInfo: reconciler.ClusterInfo{
				GVK: &metav1.GroupVersionKind{
					Group:   "fake.zncdata.dev",
					Version: "v1alpha1",
					Kind:    "TrinoCluster",
				},
				ClusterName: "fake-owner",
			},
			RoleName: "trino",
		}

		var namespace string
		ctx := context.Background()

		coordinatorRole := CoordinatorSpec{
			EnvOverrides: map[string]string{"TEST": "test"},
			Config: &TrinoConfigSpec{
				Resources: &commonsv1alpha1.ResourcesSpec{
					CPU: &commonsv1alpha1.CPUResource{
						Max: resource.Quantity{
							Format: "100m",
						},
					},
				},
			},
			RoleGroups: map[string]TrinoRoleGroupSpec{
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
				&commonsv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				coordinatorRole,
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

		roleInfo := reconciler.RoleInfo{
			ClusterInfo: reconciler.ClusterInfo{
				GVK: &metav1.GroupVersionKind{
					Group:   "fake.zncdata.dev",
					Version: "v1alpha1",
					Kind:    "TrinoCluster",
				},
				ClusterName: "fake-owner",
			},
			RoleName: "Trino",
		}

		resourceClient := client.NewClient(k8sClient, nil)
		var role *CoordinatorSpec

		BeforeEach(func() {
			role = &CoordinatorSpec{
				EnvOverrides: map[string]string{"TEST": "test"},
				Config: &TrinoConfigSpec{
					GracefulShutdownTimeout: "10s",
				},
				CommandOverrides: []string{
					"tail",
				},
				RoleGroups: map[string]TrinoRoleGroupSpec{
					"default": {
						Replicas: &[]int32{1}[0],
						CommandOverrides: []string{
							"echo",
						},
					},
					"test": {
						Replicas: &[]int32{2}[0],
						Config: &TrinoConfigSpec{
							GracefulShutdownTimeout: "20s",
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
				&commonsv1alpha1.ClusterOperationSpec{},
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
			defaultRoleGroup, ok := defaultRoleGroupValue.(*TrinoRoleGroupSpec)
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
			testRoleGroup, ok := testRoleGroupValue.(*TrinoRoleGroupSpec)
			Expect(ok).To(BeTrue())
			Expect(testRoleGroup).ToNot(BeNil())
			Expect(testRoleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
			// testRoleGroup.Config != role.Config
			Expect(testRoleGroup.Config).ToNot(Equal(role.Config))
			Expect(testRoleGroup.CommandOverrides).To(Equal(role.CommandOverrides))

		})
	})

	Context("RoleReconciler merge roleGroup spec", func() {

		roleInfo := reconciler.RoleInfo{
			ClusterInfo: reconciler.ClusterInfo{
				GVK: &metav1.GroupVersionKind{
					Group:   "fake.zncdata.dev",
					Version: "v1alpha1",
					Kind:    "TrinoCluster",
				},
				ClusterName: "fake-owner",
			},
			RoleName: "coordinator",
		}

		resourceClient := client.NewClient(k8sClient, nil)
		var role *CoordinatorSpec
		var roleGroupOne *TrinoRoleGroupSpec
		var roleGroupTwo *TrinoRoleGroupSpec

		BeforeEach(func() {
			role = &CoordinatorSpec{
				EnvOverrides: map[string]string{"TEST": "test"},
				Config: &TrinoConfigSpec{
					GracefulShutdownTimeout: "10s",
				},
				CommandOverrides: []string{
					"tail",
				},
			}

			roleGroupOne = &TrinoRoleGroupSpec{
				Replicas: &[]int32{1}[0],
				CommandOverrides: []string{
					"echo",
				},
			}

			roleGroupTwo = &TrinoRoleGroupSpec{
				Replicas: &[]int32{2}[0],
				Config: &TrinoConfigSpec{
					GracefulShutdownTimeout: "20s",
				},
			}
		})

		It("should merged role to roleGroup spec with Role.Config when RoleGroup.config not exist", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&commonsv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				*role,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("merge role group spec")
			roleGroupValue := roleReconciler.MergeRoleGroupSpec(roleGroupOne)
			Expect(roleGroupValue).ToNot(BeNil())

			By("assert roleGroupValue is TrinoRoleGroupSpec")
			roleGroup, ok := roleGroupValue.(*TrinoRoleGroupSpec)
			Expect(ok).To(BeTrue())

			By("checking role.Config merged")
			Expect(roleGroup.Config.GracefulShutdownTimeout).To(Equal(role.Config.GracefulShutdownTimeout))

			By("checking role.CommandOverrides not merged")
			Expect(roleGroup.CommandOverrides).ToNot(Equal(role.CommandOverrides))

			By("checking role.EnvOverrides merged")
			Expect(roleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
		})

		It("should not merged role to roleGroup spec with Role.Config when RoleGroup.config exist", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&commonsv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				*role,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("merge role group spec")
			roleGroupValue := roleReconciler.MergeRoleGroupSpec(roleGroupTwo)
			Expect(roleGroupValue).ToNot(BeNil())

			By("assert roleGroupValue is TrinoRoleGroupSpec")
			roleGroup, ok := roleGroupValue.(*TrinoRoleGroupSpec)
			Expect(ok).To(BeTrue())

			By("checking role.CommandOverrides merged")
			Expect(roleGroup.CommandOverrides).To(Equal(role.CommandOverrides))

			By("checking role.Config not merged")
			Expect(roleGroup.Config.GracefulShutdownTimeout).ToNot(Equal(role.Config.GracefulShutdownTimeout))

			By("checking role.EnvOverrides merged")
			Expect(roleGroup.EnvOverrides).To(Equal(role.EnvOverrides))
		})

		It("should merge itself", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				roleInfo,
				&commonsv1alpha1.ClusterOperationSpec{},
				&ClusterConfigSpec{},
				*role,
			)

			By("merge role group spec")
			roleReconciler.MergeRoleGroupSpec(roleGroupOne)

			By("checking role.Config merged")
			Expect(roleGroupOne.Config.GracefulShutdownTimeout).To(Equal(role.Config.GracefulShutdownTimeout))

			By("checking role.CommandOverrides not merged")
			Expect(roleGroupOne.CommandOverrides).ToNot(Equal(role.CommandOverrides))

			By("checking role.EnvOverrides merged")
			Expect(roleGroupOne.EnvOverrides).To(Equal(role.EnvOverrides))

		})
	})
})
