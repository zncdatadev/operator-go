/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package s3_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	s3v1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/s3/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/s3"
	"github.com/zncdatadev/operator-go/pkg/security"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const namespace = "test-ns"

func newFakeClient(objs ...ctrlclient.Object) ctrlclient.Client {
	scheme := runtime.NewScheme()
	Expect(s3v1alpha1.AddToScheme(scheme)).To(Succeed())
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func inlineConnection() *s3v1alpha1.S3ConnectionSpec {
	return &s3v1alpha1.S3ConnectionSpec{
		Host:      "minio",
		Port:      9000,
		PathStyle: true,
		Region:    "us-east-1",
		Credentials: &commonsv1alpha1.Credentials{
			SecretClass: "s3-credentials",
		},
	}
}

var _ = Describe("ResolveConnection", func() {
	It("resolves an inline connection", func() {
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, inlineConnection(), "")
		Expect(err).NotTo(HaveOccurred())

		Expect(info.Endpoint.String()).To(Equal("http://minio:9000"))
		Expect(info.PathStyle).To(BeTrue())
		Expect(info.Region).To(Equal("us-east-1"))
		Expect(info.TLSEnabled()).To(BeFalse())
		Expect(info.Credentials.SecretClass).To(Equal("s3-credentials"))
	})

	It("resolves a referenced S3Connection from the CR namespace", func() {
		conn := &s3v1alpha1.S3Connection{
			ObjectMeta: metav1.ObjectMeta{Name: "minio", Namespace: namespace},
			Spec:       *inlineConnection(),
		}
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(conn), namespace, nil, "minio")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Endpoint.Host).To(Equal("minio:9000"))
	})

	It("uses https when the connection declares TLS", func() {
		spec := inlineConnection()
		spec.Tls = &s3v1alpha1.Tls{}
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, spec, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Endpoint.Scheme).To(Equal("https"))
		Expect(info.TLSEnabled()).To(BeTrue())
	})

	It("omits the port when unset", func() {
		spec := inlineConnection()
		spec.Port = 0
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, spec, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Endpoint.String()).To(Equal("http://minio"))
	})

	It("rejects inline and reference together", func() {
		_, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, inlineConnection(), "minio")
		Expect(err).To(MatchError(ContainSubstring("mutually exclusive")))
	})

	It("rejects neither inline nor reference", func() {
		_, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, nil, "")
		Expect(err).To(MatchError(ContainSubstring("neither inline nor reference")))
	})

	It("fails on a missing referenced S3Connection", func() {
		_, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, nil, "absent")
		Expect(err).To(MatchError(ContainSubstring(`referenced S3Connection "absent"`)))
	})
})

var _ = Describe("ResolveBucket", func() {
	It("resolves an inline bucket with an inline connection", func() {
		bucket := &s3v1alpha1.S3BucketSpec{
			BucketName: "spark-history",
			Connection: &s3v1alpha1.S3BucketConnectionSpec{Inline: inlineConnection()},
		}
		info, err := s3.ResolveBucket(context.Background(), newFakeClient(), namespace, bucket, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.BucketName).To(Equal("spark-history"))
		Expect(info.Endpoint.String()).To(Equal("http://minio:9000"))
	})

	It("resolves a referenced bucket whose connection is itself a reference", func() {
		conn := &s3v1alpha1.S3Connection{
			ObjectMeta: metav1.ObjectMeta{Name: "minio", Namespace: namespace},
			Spec:       *inlineConnection(),
		}
		bucket := &s3v1alpha1.S3Bucket{
			ObjectMeta: metav1.ObjectMeta{Name: "spark-history", Namespace: namespace},
			Spec: s3v1alpha1.S3BucketSpec{
				BucketName: "spark-history",
				Connection: &s3v1alpha1.S3BucketConnectionSpec{Reference: "minio"},
			},
		}
		info, err := s3.ResolveBucket(context.Background(), newFakeClient(conn, bucket), namespace, nil, "spark-history")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.BucketName).To(Equal("spark-history"))
		Expect(info.Endpoint.Host).To(Equal("minio:9000"))
		Expect(info.Credentials.SecretClass).To(Equal("s3-credentials"))
	})

	It("fails on a bucket without a connection", func() {
		bucket := &s3v1alpha1.S3BucketSpec{BucketName: "b"}
		_, err := s3.ResolveBucket(context.Background(), newFakeClient(), namespace, bucket, "")
		Expect(err).To(MatchError(ContainSubstring("no connection")))
	})

	It("fails on a missing referenced S3Bucket", func() {
		_, err := s3.ResolveBucket(context.Background(), newFakeClient(), namespace, nil, "absent")
		Expect(err).To(MatchError(ContainSubstring(`referenced S3Bucket "absent"`)))
	})
})

var _ = Describe("BucketInfo rendering", func() {
	It("renders the s3a URI", func() {
		info := &s3.BucketInfo{BucketName: "spark-history"}
		Expect(info.S3AURI("events")).To(Equal("s3a://spark-history/events"))
	})

	It("renders S3A properties faithfully from the resolved connection", func() {
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, inlineConnection(), "")
		Expect(err).NotTo(HaveOccurred())

		props := info.S3AProperties()
		Expect(props).To(HaveKeyWithValue("fs.s3a.endpoint", "http://minio:9000"))
		Expect(props).To(HaveKeyWithValue("fs.s3a.path.style.access", "true"))
		Expect(props).To(HaveKeyWithValue("fs.s3a.connection.ssl.enabled", "false"))
		Expect(props).To(HaveKeyWithValue("fs.s3a.endpoint.region", "us-east-1"))
	})

	It("omits the region property when the region is unset", func() {
		spec := inlineConnection()
		spec.Region = ""
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, spec, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.S3AProperties()).NotTo(HaveKey("fs.s3a.endpoint.region"))
	})
})

var _ = Describe("Credentials wiring", func() {
	It("returns no provisioner for anonymous connections", func() {
		spec := inlineConnection()
		spec.Credentials = nil
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, spec, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.CredentialsProvisioner("s3-credentials")).To(BeNil())
	})

	It("provisions a plain credential CSI volume under /kubedoop/secret", func() {
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, inlineConnection(), "")
		Expect(err).NotTo(HaveOccurred())

		provisioner := info.CredentialsProvisioner(s3.DefaultCredentialsVolumeName)
		Expect(provisioner).NotTo(BeNil())

		volumes := provisioner.Volumes()
		Expect(volumes).To(HaveLen(1))
		annotations := volumes[0].VolumeSource.Ephemeral.VolumeClaimTemplate.Annotations
		Expect(annotations).To(HaveKeyWithValue(security.SecretClassAnnotation, "s3-credentials"))
		Expect(annotations).NotTo(HaveKey(security.AnnotationSecretsFormat),
			"plain credential secrets carry no format annotation")
		Expect(annotations).NotTo(HaveKey(security.SecretClassScopeAnnotation),
			"no scope annotation when the credentials declare no scope")

		mounts := provisioner.VolumeMounts()
		Expect(mounts).To(HaveLen(1))
		Expect(mounts[0].MountPath).To(Equal("/kubedoop/secret/s3-credentials"))
		Expect(s3.CredentialsMountPath(s3.DefaultCredentialsVolumeName)).To(Equal("/kubedoop/secret/s3-credentials"))
	})

	It("renders the credentials scope with the secret-operator key=value convention", func() {
		spec := inlineConnection()
		spec.Credentials.Scope = &commonsv1alpha1.CredentialsScope{
			Node:     true,
			Pod:      true,
			Services: []string{"minio"},
		}
		info, err := s3.ResolveConnection(context.Background(), newFakeClient(), namespace, spec, "")
		Expect(err).NotTo(HaveOccurred())

		provisioner := info.CredentialsProvisioner(s3.DefaultCredentialsVolumeName)
		annotations := provisioner.Volumes()[0].VolumeSource.Ephemeral.VolumeClaimTemplate.Annotations
		Expect(annotations).To(HaveKeyWithValue(security.SecretClassScopeAnnotation, "node,pod,service=minio"))
	})

	It("renders the AWS env export script", func() {
		script := s3.CredentialsExportScript(s3.CredentialsMountPath("s3-credentials"))
		Expect(script).To(ContainSubstring(`export AWS_ACCESS_KEY_ID="$(cat /kubedoop/secret/s3-credentials/ACCESS_KEY)"`))
		Expect(script).To(ContainSubstring(`export AWS_SECRET_ACCESS_KEY="$(cat /kubedoop/secret/s3-credentials/SECRET_KEY)"`))
	})
})
