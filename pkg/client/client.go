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

func (c *Client) GetCtrlClient() ctrlclient.Client {
	return c.Client
}

func (c *Client) GetCtrlScheme() *runtime.Scheme {
	return c.Client.Scheme()
}

func (c *Client) GetOwnerReference() ctrlclient.Object {
	return c.OwnerReference
}
func (c *Client) GetOwnerNamespace() string {
	return c.OwnerReference.GetNamespace()
}

func (c *Client) GetOwnerName() string {
	return c.OwnerReference.GetName()
}

// Get the object from the cluster
// If the object has no namespace, it will use the owner namespace
func (c *Client) Get(ctx context.Context, obj ctrlclient.Object) error {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = c.GetOwnerNamespace()
		clientLogger.V(5).Info(""+
			"ResourceClient.Get accept obj without namespace, try to use owner namespace",
			"namespace", namespace,
			"name", name,
		)
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

func (c *Client) SetOwnerReference(obj ctrlclient.Object, gvk *schema.GroupVersionKind) error {

	if obj.GetNamespace() == "" {
		clientLogger.V(5).Info("Skip setting owner reference for object without namespace, it maybe a cluster-scoped resource",
			"gvk", gvk,
			"name", obj.GetName(),
		)
		return nil
	}
	if err := ctrl.SetControllerReference(c.OwnerReference, obj, c.Client.Scheme()); err != nil {
		clientLogger.Error(err, "Failed to set owner reference",
			"gvk", gvk,
			"namespace", obj.GetNamespace(),
			"name", obj.GetName(),
		)
		return err
	}

	clientLogger.V(5).Info("Set owner reference for object",
		"gvk", gvk, "namespace",
		obj.GetNamespace(),
		"name", obj.GetName(),
		"owner", c.OwnerReference.GetName(),
	)

	return nil
}

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

	clientLogger.V(5).Info("Creating or updating object", "gvk", gvk, "namespace", namespace, "name", name)

	current := obj.DeepCopyObject().(ctrlclient.Object)
	// Check if the object exists, if not create a new one
	err = c.Client.Get(ctx, objectKey, current)
	var calculateOpt = []patch.CalculateOption{
		patch.IgnoreStatusFields(),
	}
	if apierrors.IsNotFound(err) {
		clientLogger.V(1).Info("Resource not found, creating a new.", "gvk", gvk, "namespace", namespace, "name", name)
		if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(obj); err != nil {
			return false, err
		}
		if err := c.Client.Create(ctx, obj); err != nil {
			clientLogger.Error(err, "Failed to create resource", "gvk", gvk, "namespace", namespace, "name", name)
			return false, err
		}
		clientLogger.V(5).Info("Resource created", "gvk", gvk, "namespace", namespace, "name", name)
		return true, nil
	} else if err == nil {
		switch obj.(type) {
		case *corev1.Service:
			currentSvc := current.(*corev1.Service)
			svc := obj.(*corev1.Service)
			// Preserve the ClusterIP when updating the service
			svc.Spec.ClusterIP = currentSvc.Spec.ClusterIP
			// Preserve the annotation when updating the service, ensure any updated annotation is preserved
			//for key, value := range currentSvc.Annotations {
			//	if _, present := svc.Annotations[key]; !present {
			//		svc.Annotations[key] = value
			//	}
			//}

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
			clientLogger.Error(err, "Failed to calculate patch to match objects, moving to update", "gvk", gvk, "namespace", namespace, "name", name)
			// if there is an error with matching, we still want to update
			resourceVersion := current.(metav1.ObjectMetaAccessor).GetObjectMeta().GetResourceVersion()
			obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetResourceVersion(resourceVersion)

			if err := c.Client.Update(ctx, obj); err != nil {
				clientLogger.Error(err, "Failed to update resource", "gvk", gvk, "namespace", namespace, "name", name)
				return false, err
			}
			clientLogger.V(5).Info("Resource updated", "gvk", gvk, "namespace", namespace, "name", name)
			return true, nil
		}

		if !result.IsEmpty() {
			clientLogger.V(1).Info("Resource modified, updating", "gvk", gvk, "namespace", namespace, "name", name)
			// ignore the update if the object is a secret
			if _, ok := obj.(*corev1.Secret); !ok {
				clientLogger.V(1).Info("Patch result", "gvk", gvk, "namespace", namespace, "name", name, "patch", string(result.Patch))
			}

			if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(obj); err != nil {
				clientLogger.Error(err, "Failed to update object annotation, moving to update", "gvk", gvk, "namespace", namespace, "name", name)
			}

			resourceVersion := current.(metav1.ObjectMetaAccessor).GetObjectMeta().GetResourceVersion()
			obj.(metav1.ObjectMetaAccessor).GetObjectMeta().SetResourceVersion(resourceVersion)

			if err = c.Client.Update(ctx, obj); err != nil {
				clientLogger.Error(err, "Failed to update resource", "gvk", gvk, "namespace", namespace, "name", name)
				return false, err
			}
			return true, nil
		}
		clientLogger.V(1).Info("Skipping update for object", "gvk", gvk, "namespace", namespace, "name", name)
	}
	return false, err
}

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
