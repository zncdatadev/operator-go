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

// Package s3 resolves the s3.kubedoop.dev connection and bucket CRDs into the concrete
// facts a product operator needs — endpoint URL, region, addressing style, TLS and
// credentials — and wires the credential delivery (secret-operator CSI volume plus AWS SDK
// env exports). Product CRDs embed inline-or-reference pairs for S3Connection/S3Bucket;
// this package owns the resolution chain so each product no longer hand-rolls it.
package s3

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	s3v1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/s3/v1alpha1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ConnectionInfo is a resolved S3 connection: every inline/reference indirection has been
// followed and the endpoint facts are ready for config rendering.
type ConnectionInfo struct {
	// Endpoint is the S3 endpoint URL. The scheme is https when the connection declares
	// TLS, http otherwise.
	Endpoint url.URL

	// Region is the signing region from the connection spec ("" when unset).
	Region string

	// PathStyle is the addressing style from the connection spec: true for path-style
	// (required by most self-hosted backends such as MinIO), false for virtual-host style.
	PathStyle bool

	// TLS carries the connection's TLS verification spec, nil when TLS is not configured.
	// CA material delivery is the product's concern for now.
	TLS *s3v1alpha1.Tls

	// Credentials names the SecretClass (and scope) delivering ACCESS_KEY/SECRET_KEY.
	// Nil when the connection is anonymous.
	Credentials *commonsv1alpha1.Credentials
}

// BucketInfo is a resolved S3 bucket: the bucket name plus its resolved connection.
type BucketInfo struct {
	ConnectionInfo

	// BucketName is the bucket to address on the connection's endpoint.
	BucketName string
}

// ResolveConnection resolves an inline-or-reference S3 connection pair into ConnectionInfo.
// Exactly one of inline and reference must be set; the referenced S3Connection is fetched
// from the given namespace.
func ResolveConnection(ctx context.Context, c ctrlclient.Client, namespace string, inline *s3v1alpha1.S3ConnectionSpec, reference string) (*ConnectionInfo, error) {
	spec, err := resolveConnectionSpec(ctx, c, namespace, inline, reference)
	if err != nil {
		return nil, err
	}
	return connectionInfoFromSpec(spec), nil
}

// ResolveBucket resolves an inline-or-reference S3 bucket pair into BucketInfo, following
// the bucket's own inline-or-reference connection. Exactly one of inline and reference must
// be set; referenced objects are fetched from the given namespace.
func ResolveBucket(ctx context.Context, c ctrlclient.Client, namespace string, inline *s3v1alpha1.S3BucketSpec, reference string) (*BucketInfo, error) {
	bucketSpec := inline
	switch {
	case inline != nil && reference != "":
		return nil, fmt.Errorf("invalid S3 bucket: inline and reference are mutually exclusive")
	case reference != "":
		bucket := &s3v1alpha1.S3Bucket{}
		if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: reference}, bucket); err != nil {
			return nil, fmt.Errorf("failed to get referenced S3Bucket %q: %w", reference, err)
		}
		bucketSpec = &bucket.Spec
	case inline == nil:
		return nil, fmt.Errorf("invalid S3 bucket: neither inline nor reference is set")
	}

	if bucketSpec.Connection == nil {
		return nil, fmt.Errorf("invalid S3 bucket %q: no connection is set", bucketSpec.BucketName)
	}
	conn, err := ResolveConnection(ctx, c, namespace, bucketSpec.Connection.Inline, bucketSpec.Connection.Reference)
	if err != nil {
		return nil, err
	}

	return &BucketInfo{
		ConnectionInfo: *conn,
		BucketName:     bucketSpec.BucketName,
	}, nil
}

// resolveConnectionSpec follows the inline-or-reference pair to a concrete connection spec.
func resolveConnectionSpec(ctx context.Context, c ctrlclient.Client, namespace string, inline *s3v1alpha1.S3ConnectionSpec, reference string) (*s3v1alpha1.S3ConnectionSpec, error) {
	switch {
	case inline != nil && reference != "":
		return nil, fmt.Errorf("invalid S3 connection: inline and reference are mutually exclusive")
	case reference != "":
		conn := &s3v1alpha1.S3Connection{}
		if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: reference}, conn); err != nil {
			return nil, fmt.Errorf("failed to get referenced S3Connection %q: %w", reference, err)
		}
		return &conn.Spec, nil
	case inline != nil:
		return inline, nil
	default:
		return nil, fmt.Errorf("invalid S3 connection: neither inline nor reference is set")
	}
}

// connectionInfoFromSpec maps a connection spec to resolved facts. The endpoint scheme
// follows the TLS declaration: https when TLS is configured, http otherwise.
func connectionInfoFromSpec(spec *s3v1alpha1.S3ConnectionSpec) *ConnectionInfo {
	scheme := "http"
	if spec.Tls != nil {
		scheme = "https"
	}
	endpoint := url.URL{
		Scheme: scheme,
		Host:   spec.Host,
	}
	if spec.Port != 0 {
		endpoint.Host += ":" + strconv.Itoa(spec.Port)
	}

	return &ConnectionInfo{
		Endpoint:    endpoint,
		Region:      spec.Region,
		PathStyle:   spec.PathStyle,
		TLS:         spec.Tls,
		Credentials: spec.Credentials,
	}
}
