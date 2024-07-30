package builder

import (
	"fmt"
	"strings"

	"github.com/zncdatadev/operator-go/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VolumeBuilder interface {
	Builde() *corev1.Volume
}

type scopeOptions struct {
	Pod            bool
	Node           bool
	Service        string
	ListenerVolume string
}

type SecretOperatorVolume struct {
	Name        string
	SecretClass string

	scope                *scopeOptions
	kerberosServiceNames []string
	formatName           constants.SecretFormat
	pkcs12Password       string
	certLifeTime         string
	certJitterFactor     string
}

func NewSecretOperatorVolume(name, secretClass string) *SecretOperatorVolume {
	return &SecretOperatorVolume{
		Name:        name,
		SecretClass: secretClass,
	}
}

func (s *SecretOperatorVolume) Builde() *corev1.Volume {
	return &corev1.Volume{
		Name: s.Name,
		VolumeSource: corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: s.getPVCAnnotations(),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: constants.SecretStorageClassPtr(),
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Mi"),
							},
						},
					},
				},
			},
		},
	}
}

func (s *SecretOperatorVolume) getPVCAnnotations() map[string]string {
	annotations := map[string]string{
		constants.AnnotationSecretsClass: s.SecretClass,
	}

	if s.scope != nil {
		var scopes []string
		if s.scope.Pod {
			scopes = append(scopes, string(constants.PodScope))
		}
		if s.scope.Node {
			scopes = append(scopes, string(constants.NodeScope))
		}
		if s.scope.Service != "" {
			scopes = append(scopes, fmt.Sprintf("%s=%s", constants.ServiceScope, s.scope.Service))
		}
		if s.scope.ListenerVolume != "" {
			scopes = append(scopes, fmt.Sprintf("%s=%s", constants.ListenerVolumeScope, s.scope.ListenerVolume))
		}
		annotations[constants.AnnotationSecretsScope] = strings.Join(scopes, ",")
	}

	if len(s.kerberosServiceNames) > 0 {
		annotations[constants.AnnotationSecretsKerberosServiceNames] = strings.Join(s.kerberosServiceNames, ",")
	}

	if s.formatName != "" {
		annotations[constants.AnnotationSecretsFormat] = string(s.formatName)
	}

	if s.pkcs12Password != "" {
		annotations[constants.AnnotationSecretsPKCS12Password] = s.pkcs12Password
	}

	if s.certLifeTime != "" {
		annotations[constants.AnnotationSecretCertLifeTime] = s.certLifeTime
	}

	if s.certJitterFactor != "" {
		annotations[constants.AnnotationSecretsCertJitterFactor] = s.certJitterFactor
	}

	return annotations
}

func (s *SecretOperatorVolume) SetScope(pod, node bool, service, listenerVolume string) {
	s.scope = &scopeOptions{
		Pod:            pod,
		Node:           node,
		Service:        service,
		ListenerVolume: listenerVolume,
	}
}

func (s *SecretOperatorVolume) SetKerberosServiceNames(service string, services ...string) {
	s.kerberosServiceNames = append([]string{service}, services...)
}

func (s *SecretOperatorVolume) SetFormatName(format constants.SecretFormat) {
	s.formatName = format
}

func (s *SecretOperatorVolume) SetPKCS12Password(password string) {
	s.pkcs12Password = password
}

func (s *SecretOperatorVolume) SetCertLifeTime(lifetime string) {
	s.certLifeTime = lifetime
}

func (s *SecretOperatorVolume) SetCertJitterFactor(factor string) {
	s.certJitterFactor = factor
}

type ListenerOperatorVolume struct {
	Name          string
	ListenerClass string

	listenerName string
}

func NewListenerOperatorVolume(name, listenerClass string) *ListenerOperatorVolume {
	return &ListenerOperatorVolume{
		Name:          name,
		ListenerClass: listenerClass,
	}
}

func (l *ListenerOperatorVolume) Builde() *corev1.Volume {
	return &corev1.Volume{
		Name: l.Name,
		VolumeSource: corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: l.getPVCAnnotations(),
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: constants.ListenerStorageClassPtr(),
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Mi"),
							},
						},
					},
				},
			},
		},
	}
}

func (l *ListenerOperatorVolume) getPVCAnnotations() map[string]string {
	annotations := map[string]string{
		constants.AnnotationListenersClass: l.ListenerClass,
	}

	if l.listenerName != "" {
		annotations[constants.AnnotationListenerName] = l.listenerName
	}

	return annotations
}

func (l *ListenerOperatorVolume) SetListenerName(name string) {
	l.listenerName = name
}
