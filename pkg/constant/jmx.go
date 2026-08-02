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

import "strconv"

// The Prometheus JMX exporter, as the kubedoop JVM images actually ship it.
//
// Every kubedoop image carrying the exporter installs the versioned jar under KubedoopJmxDir and
// symlinks it to the unversioned name below, so a product operator can reference it without
// knowing the version. The exporter runs as a JAVA AGENT inside the product's own JVM.
//
// This is NOT what pkg/sidecar/jmx_exporter.go models. That provider runs
// `jmx_prometheus_httpserver.jar` from `/opt/jmx_exporter` as a separate container, which is a
// different deployment of the same upstream project and a path no kubedoop image contains. The
// framework abstracted the variant nobody deploys and left the one everybody deploys as a string
// literal in six operators; these constants close that half.
const (
	// KubedoopJmxAgentJar is the Prometheus JMX exporter java agent, at the unversioned symlink
	// the images provide.
	KubedoopJmxAgentJar = KubedoopJmxDir + "jmx_prometheus_javaagent.jar"
)

// JMXJavaAgentOpt renders the `-javaagent` JVM option that starts the exporter on the given port
// with the given config file, which is resolved relative to KubedoopJmxDir:
//
//	constant.JMXJavaAgentOpt(8081, "config.yaml")
//	// -javaagent:/kubedoop/jmx/jmx_prometheus_javaagent.jar=8081:/kubedoop/jmx/config.yaml
//
// # Why the config file is a parameter rather than a constant
//
// It is tempting to bake in "config.yaml", and five of the six operators that hand-build this
// string would be satisfied by that. HDFS would not: the hadoop image ships no config.yaml at all,
// only namenode.yaml, datanode.yaml and journalnode.yaml, because the metrics worth exporting
// differ per role. A helper that could not express that would have excluded the one product with
// the most roles, so the file name stays the caller's to choose.
//
// The port is the caller's too. It is the port the agent listens on for scrapes, so it must match
// whatever container port and metrics Service the product declares; the framework has no way to
// know it.
func JMXJavaAgentOpt(port int32, configFile string) string {
	return "-javaagent:" + KubedoopJmxAgentJar + "=" + strconv.Itoa(int(port)) + ":" + KubedoopJmxDir + configFile
}
