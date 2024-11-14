package builder_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
)

func TestNewServiceBuilder(t *testing.T) {
	fakeOwner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-owner",
			Namespace: "default",
			UID:       types.UID("fake-uid"),
		},
	}

	mockClient := client.NewClient(k8sClient, fakeOwner)
	name := "test-service"
	ports := []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: 80,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	t.Run("default options", func(t *testing.T) {
		builder := builder.NewServiceBuilder(mockClient, name, ports)
		service := builder.GetObject()

		assert.Equal(t, name, service.Name)
		assert.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)
		assert.Equal(t, 1, len(service.Spec.Ports))
		assert.Equal(t, "http", service.Spec.Ports[0].Name)
		assert.Equal(t, int32(80), service.Spec.Ports[0].Port)
		assert.Equal(t, corev1.ProtocolTCP, service.Spec.Ports[0].Protocol)

		obj, err := builder.Build(context.Background())
		assert.Nil(t, err)
		assert.NotNil(t, obj)
	})

	t.Run("with options", func(t *testing.T) {
		options := []builder.ServiceBuilderOption{
			func(opt *builder.ServiceBuilderOptions) {
				opt.ListenerClass = constants.ExternalStable
				opt.Headless = true
				opt.MatchingLabels = map[string]string{"app": "test"}
			},
		}
		builder := builder.NewServiceBuilder(mockClient, name, ports, options...)
		service := builder.GetObject()

		assert.Equal(t, name, service.Name)
		assert.Equal(t, corev1.ServiceTypeLoadBalancer, service.Spec.Type)
		assert.Equal(t, corev1.ClusterIPNone, service.Spec.ClusterIP)
		assert.Equal(t, map[string]string{"app": "test"}, service.Spec.Selector)

		obj, err := builder.Build(context.Background())
		assert.Nil(t, err)
		assert.NotNil(t, obj)
	})
}
