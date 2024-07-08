package client

import (
	"context"
	"fmt"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	clientLogger = ctrl.Log.WithName("client")
)

type Client struct {
	Client ctrlclient.Client

	OwnerReference ctrlclient.Object
}

// NewClient returns a new instance of the Client struct.
// It accepts a control client `client` and an owner reference `ownerReference`.
func NewClient(client ctrlclient.Client, ownerReference ctrlclient.Object) *Client {
	return &Client{
		Client:         client,
		OwnerReference: ownerReference,
	}
}

// GetCtrlClient returns the control client associated with the client.
func (c *Client) GetCtrlClient() ctrlclient.Client {
	return c.Client
}

// GetCtrlScheme returns the control scheme used by the client.
func (c *Client) GetCtrlScheme() *runtime.Scheme {
	return c.Client.Scheme()
}

// GetOwnerReference returns the owner reference of the client.
func (c *Client) GetOwnerReference() ctrlclient.Object {
	return c.OwnerReference
}

// GetOwnerNamespace returns the namespace of the owner reference.
func (c *Client) GetOwnerNamespace() string {
	return c.OwnerReference.GetNamespace()
}

// GetOwnerName returns the name of the owner reference.
func (c *Client) GetOwnerName() string {
	return c.OwnerReference.GetName()
}

// Get retrieves the object specified by the given `obj` from the Kubernetes cluster.
// It accepts a context `ctx` for cancellation and a `obj` of type `ctrlclient.Object` that represents the object to be retrieved.
// If the `obj` does not have a namespace specified, it uses the owner's namespace.
// It returns an error if the retrieval fails, along with additional information about the resource.
// Parameters:
//   - ctx: The context for the operation.
//   - obj: The object to retrieve.
//
// Returns:
//   - error: An error if the operation fails, otherwise nil.
func (c *Client) Get(ctx context.Context, obj ctrlclient.Object) error {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = c.GetOwnerNamespace()
		clientLogger.V(1).Info("ResourceClient.Get accept obj without namespace, try to use owner namespace", "namespace", namespace, "name", name)
	}
	kind := obj.GetObjectKind()
	if err := c.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		opt := []any{"ns", namespace, "name", name, "kind", kind}
		if apierrors.IsNotFound(err) {
			clientLogger.V(0).Info("Fetch resource NotFound", opt...)
		} else {
			clientLogger.Error(err, "Fetch resource occur some unknown err", opt...)
		}
		return err
	}
	return nil
}

// SetOwnerReference sets the owner reference for the given object.
// If the object doesn't have a namespace, it skips setting the owner reference and returns nil.
// If the client's OwnerReference is nil, it skips setting the owner reference and returns nil.
// Otherwise, it sets the owner reference using the ctrl.SetControllerReference function.
// It logs an error if setting the owner reference fails.
// Finally, it logs a message indicating that the owner reference has been set for the object.
// Parameters:
//   - obj: The object for which to set the owner reference.
//   - gvk: The GroupVersionKind of the object.
//
// Returns:
//   - error: An error if setting the owner reference fails, otherwise nil.
func (c *Client) SetOwnerReference(obj ctrlclient.Object, gvk *schema.GroupVersionKind) error {

	if obj.GetNamespace() == "" {
		clientLogger.V(1).Info("Skip setting owner reference for object without namespace, it maybe a cluster-scoped resource", "gvk", gvk, "name", obj.GetName())
		return nil
	}

	if c.OwnerReference == nil {
		clientLogger.V(1).Info("Skip setting owner reference for object without owner reference", "gvk", gvk, "namespace", obj.GetNamespace(), "name", obj.GetName())
		return nil
	}

	if err := ctrl.SetControllerReference(c.OwnerReference, obj, c.Client.Scheme()); err != nil {
		clientLogger.Error(err, "Failed to set owner reference", "gvk", gvk, "namespace", obj.GetNamespace(), "name", obj.GetName())
		return err
	}

	clientLogger.V(5).Info("Set owner reference for object", "gvk", gvk, "namespace", obj.GetNamespace(), "name", obj.GetName(), "owner", c.OwnerReference.GetName())

	return nil
}

// CreateOrUpdate creates or updates an object in the Kubernetes cluster.
// It takes a context and an object that implements the `ctrlclient.Object` interface.
// The function returns a boolean indicating whether a mutation occurred and an error, if any.
// If the object does not exist, a new one is created. If the object already exists, it is updated.
// The function first checks if the object exists by calling `Get` on the client.
// If the object does not exist, it creates a new one by calling `Create` on the client.
// If the object exists, it calculates the patch to match the current and desired objects using `patch.DefaultPatchMaker.Calculate`.
// If the patch is not empty, it updates the object by calling `Update` on the client.
// The function also handles specific logic for `Service` and `StatefulSet` objects.
// For `Service` objects, it preserves the `ClusterIP` and annotations when updating.
// For `StatefulSet` objects, it ignores the `VolumeClaimTemplate` type metadata and status when calculating the patch.
// The function sets the `LastAppliedAnnotation` on the object using `patch.DefaultAnnotator.SetLastAppliedAnnotation`.
// It also sets the `ResourceVersion` of the object to match the current object before updating.
// The function returns `true` if a mutation occurred, indicating that the object was created or updated.
// If no mutation occurred, it returns `false`.
// If an error occurs during any step, the function returns the error.
// Parameters:
//   - ctx: The context for the operation.
//   - obj: The object to create or update.
//
// Returns:
//   - mutation: A boolean indicating whether a mutation occurred.
//   - error: An error if the operation fails, otherwise nil.
func (c *Client) CreateOrUpdate(ctx context.Context, obj ctrlclient.Object) (mutation bool, err error) {

	objectKey := ctrlclient.ObjectKeyFromObject(obj)
	namespace := obj.GetNamespace()

	gvk, err := GetObjectGVK(obj)
	if err != nil {
		return false, err
	}

	name := obj.GetName()

	if err := c.SetOwnerReference(obj, gvk); err != nil {
		return false, err
	}

	logExtraValues := []any{"gvk", gvk, "namespace", namespace, "name", name}

	clientLogger.V(1).Info("Creating or updating object", logExtraValues...)

	current := obj.DeepCopyObject().(ctrlclient.Object)
	// Check if the object exists, if not create a new one
	err = c.Client.Get(ctx, objectKey, current)
	var calculateOpt = []patch.CalculateOption{
		patch.IgnoreStatusFields(),
	}
	if apierrors.IsNotFound(err) {
		clientLogger.V(1).Info("Resource not found, creating a new.", logExtraValues...)
		if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(obj); err != nil {
			return false, err
		}
		if err := c.Client.Create(ctx, obj); err != nil {
			clientLogger.Error(err, "Failed to create resource", logExtraValues...)
			return false, err
		}
		clientLogger.V(1).Info("Resource created", logExtraValues...)
		return true, nil
	} else if err == nil {
		switch obj.(type) {
		case *corev1.Service:
			currentSvc := current.(*corev1.Service)
			svc := obj.(*corev1.Service)
			// Preserve the ClusterIP when updating the service
			svc.Spec.ClusterIP = currentSvc.Spec.ClusterIP
			// Preserve the annotation when updating the service, ensure any updated annotation is preserved
			// for key, value := range currentSvc.Annotations {
			// 	if _, present := svc.Annotations[key]; !present {
			// 		svc.Annotations[key] = value
			// 	}
			// }

			if svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				for i := range svc.Spec.Ports {
					svc.Spec.Ports[i].NodePort = currentSvc.Spec.Ports[i].NodePort
				}
			}
		case *appsv1.StatefulSet:
			calculateOpt = append(calculateOpt, patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus())
		}

		result, err := patch.DefaultPatchMaker.Calculate(current, obj, calculateOpt...)
		if err != nil {
			clientLogger.Error(err, "Failed to calculate patch to match objects, moving to update", logExtraValues...)
			// if there is an error with matching, we still want to update
			resourceVersion := current.(metav1.ObjectMetaAccessor).GetObjectMeta().GetResourceVersion()
			obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetResourceVersion(resourceVersion)

			if err := c.Client.Update(ctx, obj); err != nil {
				clientLogger.Error(err, "Failed to update resource", logExtraValues...)
				return false, err
			}
			clientLogger.V(1).Info("Resource updated", logExtraValues...)
			return true, nil
		}

		if !result.IsEmpty() {
			clientLogger.V(1).Info("Resource modified, updating", logExtraValues...)
			// TODO: Add debug flag to log Secret patch data
			// Ignore logging secret data in patch
			if _, ok := obj.(*corev1.Secret); ok {
				clientLogger.V(1).Info("Patch result", "gvk", gvk, "namespace", namespace, "name", name, "patch", "secret data is omitted")
			}
			clientLogger.V(1).Info("Patch result", "gvk", gvk, "namespace", namespace, "name", name, "patch", string(result.Patch))

			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(obj); err != nil {
				clientLogger.Error(err, "Failed to update object annotation, moving to update", logExtraValues...)
			}

			resourceVersion := current.(metav1.ObjectMetaAccessor).GetObjectMeta().GetResourceVersion()
			obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetResourceVersion(resourceVersion)

			if err = c.Client.Update(ctx, obj); err != nil {
				clientLogger.Error(err, "Failed to update resource", logExtraValues...)
				return false, err
			}
			return true, nil
		}
		clientLogger.V(1).Info("Skipping update for object", logExtraValues...)
	}
	return false, err
}

// GetObjectGVK returns the GroupVersionKind (GVK) of the provided object.
// It retrieves the GVK by using the scheme.Scheme.ObjectKinds function.
// If the GVK is not found or there is an error retrieving it, an error is returned.
func GetObjectGVK(obj ctrlclient.Object) (*schema.GroupVersionKind, error) {
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}

	if len(gvks) == 0 {
		return nil, fmt.Errorf("no GroupVersionKind found for object %T", obj)
	}

	gvk := gvks[0]

	return &gvk, nil
}
