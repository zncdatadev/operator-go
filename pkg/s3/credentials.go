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
	"path"

	"github.com/zncdatadev/operator-go/pkg/constant"
	"github.com/zncdatadev/operator-go/pkg/security"
)

const (
	// AccessKeyFile and SecretKeyFile are the file names the secret-operator serves for a
	// credential SecretClass; the s3.kubedoop.dev CRDs document these keys.
	AccessKeyFile = "ACCESS_KEY"
	SecretKeyFile = "SECRET_KEY"

	// DefaultCredentialsVolumeName is the conventional volume (and mount subdirectory)
	// name for S3 credentials.
	DefaultCredentialsVolumeName = "s3-credentials"
)

// CredentialsProvisioner returns a SecretProvisioner delivering this connection's
// credentials as a secret-operator CSI volume, mounted at CredentialsMountPath(volumeName)
// (the platform's /kubedoop/secret/<volume> convention). It satisfies
// reconciler.VolumeProvider, so it can be appended to RoleGroupBuildContext.VolumeProviders
// as-is. Returns nil when the connection carries no credentials (anonymous access) — a nil
// provisioner must simply not be registered.
func (c *ConnectionInfo) CredentialsProvisioner(volumeName string) *security.SecretProvisioner {
	if c.Credentials == nil {
		return nil
	}
	registration := security.CredentialsVolume(volumeName, c.Credentials.SecretClass)
	if scope := security.ScopeString(c.Credentials.Scope); scope != "" {
		registration = registration.WithScope(scope)
	}
	return security.NewSecretProvisioner().
		WithMountBasePath(constant.KubedoopSecretDir).
		Register(registration)
}

// CredentialsMountPath returns the mount path (no trailing slash) for a credentials volume
// created by CredentialsProvisioner.
func CredentialsMountPath(volumeName string) string {
	return path.Join(constant.KubedoopSecretDir, volumeName)
}

// CredentialsExportScript returns a shell fragment exporting the mounted credential files
// as the AWS SDK environment variables, for splicing into a container start script ahead of
// the product launch command:
//
//	export AWS_ACCESS_KEY_ID="$(cat /kubedoop/secret/<volume>/ACCESS_KEY)"
//	export AWS_SECRET_ACCESS_KEY="$(cat /kubedoop/secret/<volume>/SECRET_KEY)"
func CredentialsExportScript(mountPath string) string {
	return `export AWS_ACCESS_KEY_ID="$(cat ` + path.Join(mountPath, AccessKeyFile) + `)"
export AWS_SECRET_ACCESS_KEY="$(cat ` + path.Join(mountPath, SecretKeyFile) + `)"`
}
