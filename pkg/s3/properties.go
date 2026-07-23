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

package s3

import (
	"net/url"
	"strconv"
)

// S3AProperties renders the Hadoop S3A client properties for this connection, faithfully
// reflecting the resolved spec: endpoint, addressing style, SSL flag, and (when set) the
// signing region. Products merge these into their config files, prefixing as their product
// requires (e.g. Spark prefixes each key with "spark.hadoop."). Keys a product needs to
// pin differently (e.g. forcing path-style for a backend that requires it) can simply be
// overwritten after merging.
func (c *ConnectionInfo) S3AProperties() map[string]string {
	props := map[string]string{
		"fs.s3a.endpoint":               c.Endpoint.String(),
		"fs.s3a.path.style.access":      strconv.FormatBool(c.PathStyle),
		"fs.s3a.connection.ssl.enabled": strconv.FormatBool(c.TLSEnabled()),
	}
	if c.Region != "" {
		props["fs.s3a.endpoint.region"] = c.Region
	}
	return props
}

// TLSEnabled reports whether the connection uses TLS (https endpoint).
func (c *ConnectionInfo) TLSEnabled() bool {
	return c.TLS != nil
}

// S3AURI renders an "s3a://<bucket>/<prefix>" URI for the bucket, e.g. for Spark's
// spark.history.fs.logDirectory or any Hadoop-compatible input path.
func (b *BucketInfo) S3AURI(prefix string) string {
	uri := url.URL{
		Scheme: "s3a",
		Host:   b.BucketName,
		Path:   prefix,
	}
	return uri.String()
}
