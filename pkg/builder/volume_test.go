package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zncdatadev/operator-go/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestSecretOperatorVolume_getPVCAnnotations(t *testing.T) {
	// Test default annotations
	vol := NewSecretOperatorVolume("test-volume", "test-class")
	annotations := vol.getPVCAnnotations()
	assert.Equal(t, "test-class", annotations[constants.AnnotationSecretsClass])

	// Test with scope options
	vol.SetScope(true, false, "test-service", "test-listener")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "test-class", annotations[constants.AnnotationSecretsClass])
	assert.Equal(t, "pod,service=test-service,listener-volume=test-listener", annotations[constants.AnnotationSecretsScope])

	// Test with scope options
	vol.SetScope(false, true, "", "test-listener")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "node,listener-volume=test-listener", annotations[constants.AnnotationSecretsScope])

	// Test with Kerberos service names
	vol.SetKerberosServiceNames("service1", "service2")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "service1,service2", annotations[constants.AnnotationSecretsKerberosServiceNames])

	// Test with format name
	vol.SetFormatName(constants.SecretFormat("test-format"))
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "test-format", annotations[constants.AnnotationSecretsFormat])

	// Test with PKCS12 password
	vol.SetPKCS12Password("test-password")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "test-password", annotations[constants.AnnotationSecretsPKCS12Password])

	// Test with certificate lifetime
	vol.SetCertLifeTime("24h")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "24h", annotations[constants.AnnotationSecretCertLifeTime])

	// Test with certificate jitter factor
	vol.SetCertJitterFactor("0.5")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "0.5", annotations[constants.AnnotationSecretsCertJitterFactor])
}

func TestListenerOperatorVolume_getPVCAnnotations(t *testing.T) {
	// Test default annotations
	vol := NewListenerOperatorVolume("test-volume", "test-class")
	annotations := vol.getPVCAnnotations()
	assert.Equal(t, "test-class", annotations[constants.AnnotationListenersClass])

	// Test with listener name
	vol.SetListenerName("test-listener")
	annotations = vol.getPVCAnnotations()
	assert.Equal(t, "test-listener", annotations[constants.AnnotationListenerName])
}
func TestSecretOperatorVolume_Builde(t *testing.T) {
	vol := NewSecretOperatorVolume("test-volume", "test-class")
	vol.SetScope(true, false, "test-service", "test-listener")
	volume := vol.Builde()

	assert.Equal(t, "test-volume", volume.Name)
	assert.NotNil(t, volume.VolumeSource.Ephemeral)
	assert.NotNil(t, volume.VolumeSource.Ephemeral.VolumeClaimTemplate)
	assert.Equal(t, constants.SecretStorageClassPtr(), volume.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName)
	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, volume.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.AccessModes)
	assert.Equal(t, resource.MustParse("1Mi"), volume.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage])

	annonations := volume.VolumeSource.Ephemeral.VolumeClaimTemplate.ObjectMeta.Annotations
	assert.NotNil(t, annonations)
	assert.Equal(t, "test-class", annonations[constants.AnnotationSecretsClass])
	assert.Equal(t, "pod,service=test-service,listener-volume=test-listener", annonations[constants.AnnotationSecretsScope])
}

func TestListenerOperatorVolume_Builde(t *testing.T) {
	vol := NewListenerOperatorVolume("test-volume", "test-class")
	vol.SetListenerName("test-listener")
	volume := vol.Builde()

	assert.Equal(t, "test-volume", volume.Name)
	assert.NotNil(t, volume.VolumeSource.Ephemeral)
	assert.NotNil(t, volume.VolumeSource.Ephemeral.VolumeClaimTemplate)
	assert.Equal(t, constants.ListenerStorageClassPtr(), volume.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName)
	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, volume.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.AccessModes)
	assert.Equal(t, resource.MustParse("1Mi"), volume.VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage])

	annonations := volume.VolumeSource.Ephemeral.VolumeClaimTemplate.ObjectMeta.Annotations
	assert.NotNil(t, annonations)
	assert.Equal(t, "test-class", annonations[constants.AnnotationListenersClass])
	assert.Equal(t, "test-listener", annonations[constants.AnnotationListenerName])
}
