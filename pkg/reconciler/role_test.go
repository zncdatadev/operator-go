package reconciler_test

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

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
	reconciler.BaseRoleReconciler[TrinoCoordinatorSpec]
	ClusterConfig *ClusterConfigSpec // add more fields in implementation
}

func NewRoleReconciler(
	client *client.Client,
	clusterConfig *ClusterConfigSpec,
	clusterStopped bool,
	roleInfo reconciler.RoleInfo,
	spec TrinoCoordinatorSpec,
) *RoleReconciler {
	return &RoleReconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler(
			client,
			clusterStopped,
			roleInfo,
			spec,
		),
		ClusterConfig: clusterConfig,
	}
}

// RegisterResources registers resources with T
func (r *RoleReconciler) RegisterResources(ctx context.Context) error {
	for roleGroupName, roleGroup := range r.Spec.RoleGroups {
		// It accepts struct or pointer to struct
		// If pass pointer to struct, it will handles nil pointer.
		// 	- if original is nil, it will return override
		// 	- if override is nil, it will return original
		// 	- if both are nil, it will return nil
		mergedConfig, err := util.MergeObject(r.Spec.Config, roleGroup.Config)
		if err != nil {
			return err
		}

		overrides, err := util.MergeObject(r.Spec.OverridesSpec, roleGroup.OverridesSpec)
		if err != nil {
			return err
		}

		info := reconciler.RoleGroupInfo{
			RoleInfo:      r.RoleInfo,
			RoleGroupName: roleGroupName,
		}

		reconcilers := r.getResourceWithRoleGroup(info, mergedConfig, overrides, roleGroup.Replicas)

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
			roleLogger.Info("register resource", "role", r.GetName(), "roleGroup", roleGroupName, "reconciler", reconciler.GetName())
		}
	}
	return nil
}

func (r *RoleReconciler) getResourceWithRoleGroup(
	info reconciler.RoleGroupInfo,
	config *TrinoConfigSpec,
	overrides *commonsv1alpha1.OverridesSpec,
	replicas *int32,
) []reconciler.Reconciler {

	reconcilers := make([]reconciler.Reconciler, 0, 2)

	reconcilers = append(reconcilers, r.getServiceReconciler(info))

	deploymentReconciler := r.getDeployment(info, config, overrides, replicas)

	reconcilers = append(reconcilers, deploymentReconciler)

	return reconcilers
}

func (r *RoleReconciler) getDeployment(
	info reconciler.RoleGroupInfo,
	config *TrinoConfigSpec,
	overrides *commonsv1alpha1.OverridesSpec,
	replicas *int32,
) reconciler.Reconciler {

	var roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec

	if config != nil {
		roleGroupConfig = &config.RoleGroupConfigSpec
	}

	// Create a deployment builder
	deploymentBuilder := &TrinoCoordinatorDeploymentBuilder{
		Deployment: *builder.NewDeployment(
			r.GetClient(),
			info.GetFullName(),
			replicas,
			&util.Image{
				KubedoopVersion: "1.0.0",
				ProductName:     "trino",
				ProductVersion:  "458",
			},
			overrides,
			roleGroupConfig,
			func(o *builder.Options) {
				o.ClusterName = info.ClusterName
				o.RoleName = info.RoleName
				o.RoleGroupName = info.RoleGroupName
				o.Labels = info.GetLabels()
				o.Annotations = info.GetAnnotations()
			},
		),
	}
	// Create a deployment reconciler
	return reconciler.NewDeployment(r.Client, deploymentBuilder, r.ClusterStopped())
}

func (r *RoleReconciler) getServiceReconciler(info reconciler.RoleGroupInfo) reconciler.Reconciler {
	return reconciler.NewServiceReconciler(
		r.GetClient(),
		info.GetFullName(),
		[]corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 3000,
			},
		},
		func(o *builder.ServiceBuilderOptions) {
			o.ClusterName = info.ClusterName
			o.RoleName = info.RoleName
			o.RoleGroupName = info.RoleGroupName
			o.Labels = info.GetLabels()
			o.Annotations = info.GetAnnotations()
		},
	)
}

var _ = Describe("Role reconciler", func() {

	clusterOperation := &commonsv1alpha1.ClusterOperationSpec{
		Stopped: false,
	}

	roleInfo := reconciler.RoleInfo{
		ClusterInfo: reconciler.ClusterInfo{
			GVK: &metav1.GroupVersionKind{
				Group:   "fake.kubedoop.dev",
				Version: "v1alpha1",
				Kind:    "TrinoCluster",
			},
			ClusterName: "fake-owner",
		},
		RoleName: "coordinator",
	}

	Context("RoleReconciler test success", func() {
		var resourceClient *client.Client

		var namespace *corev1.Namespace
		var fakeOwner *corev1.ServiceAccount
		ctx := context.Background()

		coordinatorRole := TrinoCoordinatorSpec{
			RoleGroups: map[string]TrinoRoleGroupSpec{
				"default": {
					Replicas: ptr.To[int32](1),
				},
			},
		}

		BeforeEach(func() {
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-" + strconv.Itoa(rand.Intn(10000)),
				},
			}

			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
			fakeOwner = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fake-owner",
					Namespace: namespace.GetName(),
				},
			}
			resourceClient = client.NewClient(k8sClient, fakeOwner)

			Expect(resourceClient.Client.Create(ctx, fakeOwner)).Should(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, fakeOwner)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).Should(Succeed())
		})

		It("should reconcile role resource", func() {
			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				&ClusterConfigSpec{},
				clusterOperation.Stopped,
				roleInfo,
				coordinatorRole,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("registering resources")
			Expect(roleReconciler.RegisterResources(ctx)).To(Succeed())

			By("reconciling resources")
			Eventually(func() bool {
				result, err := roleReconciler.Reconcile(ctx)
				return result.IsZero() && err == nil
			}, time.Second*10, time.Second*1).Should(BeTrue())

			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.GetName(), Name: roleInfo.GetFullName() + "-default"}, deployment)).Should(Succeed())

			By("mock deployment is ready")
			deployment.Status.Replicas = 1
			deployment.Status.ReadyReplicas = 1
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())

			By("check resource until ready")
			Eventually(func() bool {
				result, err := roleReconciler.Ready(ctx)
				return result.IsZero() && err == nil
			}, time.Second*10, time.Second*1).Should(BeTrue())
		})

		It("should reconcile role pdb", func() {
			coordinatorRole := TrinoCoordinatorSpec{
				RoleGroups: map[string]TrinoRoleGroupSpec{
					"default": {Replicas: ptr.To[int32](3)},
				},
				RoleConfig: &commonsv1alpha1.RoleConfigSpec{
					PodDisruptionBudget: &commonsv1alpha1.PodDisruptionBudgetSpec{
						Enabled:        true,
						MaxUnavailable: ptr.To[int32](1),
					},
				},
			}

			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				&ClusterConfigSpec{},
				clusterOperation.Stopped,
				roleInfo,
				coordinatorRole,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("registering resources")
			Expect(roleReconciler.RegisterResources(ctx)).To(Succeed())

			By("reconciling resources")
			Eventually(func() bool {
				result, err := roleReconciler.Reconcile(ctx)
				return result.IsZero() && err == nil
			}, time.Second*10, time.Second*1).Should(BeTrue())

			By("check pdb resource with role")
			pdb := &policyv1.PodDisruptionBudget{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.GetName(), Name: roleInfo.GetFullName()}, pdb)).Should(Succeed())
			Expect(pdb.Spec.MaxUnavailable.IntVal).To(Equal(int32(1)))
		})

		It("should reconcile role resource with overrides", func() {
			coordinatorRole = TrinoCoordinatorSpec{
				OverridesSpec: &commonsv1alpha1.OverridesSpec{
					EnvOverrides: map[string]string{"test1": "test1", "test2": "test2"},
					CliOverrides: []string{"test1"},
					// ConfigOverrides: map[string]map[string]string{
					// 	"hdfs-site.xml": {"test1": "test1", "test2": "test2"},
					// 	"core-site.xml": {"test1": "test1"},
					// },
				},

				RoleGroups: map[string]TrinoRoleGroupSpec{
					"default": {
						Replicas: ptr.To[int32](1),
						OverridesSpec: &commonsv1alpha1.OverridesSpec{
							EnvOverrides: map[string]string{"test1": "test11", "test3": "test3"},
							CliOverrides: []string{"test2"},
							// ConfigOverrides: map[string]map[string]string{
							// 	"hdfs-site.xml":   {"test1": "test11", "test3": "test3"},
							// 	"mapred-site.xml": {"test1": "test11"},
							// },
						},
					},
				},
			}

			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				&ClusterConfigSpec{},
				clusterOperation.Stopped,
				roleInfo,
				coordinatorRole,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("registering resources")
			Expect(roleReconciler.RegisterResources(ctx)).To(Succeed())

			By("reconciling resources")
			Eventually(func() bool {
				result, err := roleReconciler.Reconcile(ctx)
				return result.IsZero() && err == nil
			}).WithTimeout(time.Second * 20).WithPolling(time.Second * 3).Should(BeTrue())

			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.GetName(), Name: roleInfo.GetFullName() + "-default"}, deployment)).Should(Succeed())

			By("check env overrides")
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "test1", Value: "test11"}))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "test2", Value: "test2"}))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "test3", Value: "test3"}))

			By("check cli overrides")
			Expect(container.Command).To(ContainElement("test2"))

		})

		It("should reconcile role resource with config", func() {
			coordinatorRole = TrinoCoordinatorSpec{
				Config: &TrinoConfigSpec{
					RoleGroupConfigSpec: commonsv1alpha1.RoleGroupConfigSpec{
						Resources: &commonsv1alpha1.ResourcesSpec{
							CPU: &commonsv1alpha1.CPUResource{
								Max: resource.MustParse("100m"),
							},
						},
						GracefulShutdownTimeout: "10s",
					},
				},
				RoleGroups: map[string]TrinoRoleGroupSpec{
					"default": {
						Replicas: ptr.To[int32](1),
						Config: &TrinoConfigSpec{
							RoleGroupConfigSpec: commonsv1alpha1.RoleGroupConfigSpec{
								Resources: &commonsv1alpha1.ResourcesSpec{
									CPU: &commonsv1alpha1.CPUResource{
										Max: resource.MustParse("200m"),
										Min: resource.MustParse("50m"),
									},
								},
							},
						},
					},
				},
			}

			By("creating a role reconciler")
			roleReconciler := NewRoleReconciler(
				resourceClient,
				&ClusterConfigSpec{},
				clusterOperation.Stopped,
				roleInfo,
				coordinatorRole,
			)
			Expect(roleReconciler).ToNot(BeNil())

			By("registering resources")
			Expect(roleReconciler.RegisterResources(ctx)).To(Succeed())

			By("reconciling resources")
			Eventually(func() bool {
				result, err := roleReconciler.Reconcile(ctx)
				return result.IsZero() && err == nil
			}, time.Second*10, time.Second*1).Should(BeTrue())

			deployment := &appv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.GetName(), Name: roleInfo.GetFullName() + "-default"}, deployment)).Should(Succeed())

			podSpec := deployment.Spec.Template.Spec
			container := podSpec.Containers[0]

			By("check role group config")
			Expect(podSpec.TerminationGracePeriodSeconds).ToNot(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).To(Equal(int64(10)))
			Expect(container.Resources.Limits).To(HaveKey(corev1.ResourceCPU))
			Expect(container.Resources.Limits.Cpu().String()).To(Equal("200m"))
			Expect(container.Resources.Requests).To(HaveKey(corev1.ResourceCPU))
			Expect(container.Resources.Requests.Cpu().String()).To(Equal("50m"))
		})
	})
})
