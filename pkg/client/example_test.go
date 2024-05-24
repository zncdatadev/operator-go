package client

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ExampleClient_Get() {
	client := &Client{}

	// Get a service in the same namespace as the owner object
	svcInOwnerNamespace := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-svc"},
	}

	if err := client.Get(context.Background(), svcInOwnerNamespace); err != nil {
		return
	}

	// Get a service in another namespace
	svcInAnotherNamespace := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "another-ns"},
	}
	if err := client.Get(context.Background(), svcInAnotherNamespace); err != nil {
		return
	}

}
