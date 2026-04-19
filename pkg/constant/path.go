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

package constant

// Kubedoop directory path constants used across all product operators.
// These paths define the standard directory layout inside containers.
const (
	KubedoopRoot = "/kubedoop/"

	KubedoopKerberosDir    = KubedoopRoot + "kerberos/"
	KubedoopTlsDir         = KubedoopRoot + "tls/"
	KubedoopListenerDir    = KubedoopRoot + "listener/"
	KubedoopJmxDir         = KubedoopRoot + "jmx/"
	KubedoopSecretDir      = KubedoopRoot + "secret/"
	KubedoopDataDir        = KubedoopRoot + "data/"
	KubedoopConfigDir      = KubedoopRoot + "config/"
	KubedoopLogDir         = KubedoopRoot + "log/"
	KubedoopConfigDirMount = KubedoopRoot + "mount/config/"
	KubedoopLogDirMount    = KubedoopRoot + "mount/log/"

	// KubedoopMountDir is the canonical base directory for all CSI secret volume mounts.
	// No trailing slash — consumers compose paths via fmt.Sprintf("%s/keystore.p12", path).
	KubedoopMountDir = "/kubedoop/mount"
)
