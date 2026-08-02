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

package constant_test

import (
	"testing"

	"github.com/zncdatadev/operator-go/pkg/constant"
)

func TestJMXJavaAgentOpt(t *testing.T) {
	// The exact string six operators hand-build today. It is asserted literally because the whole
	// value of the helper is that every product emits the same bytes.
	got := constant.JMXJavaAgentOpt(8081, "config.yaml")
	want := "-javaagent:/kubedoop/jmx/jmx_prometheus_javaagent.jar=8081:/kubedoop/jmx/config.yaml"
	if got != want {
		t.Errorf("JMXJavaAgentOpt(8081, %q) = %q, want %q", "config.yaml", got, want)
	}
}

func TestJMXJavaAgentOptPerRoleConfig(t *testing.T) {
	// HDFS is why the config file is a parameter: the hadoop image ships no config.yaml, only
	// per-role files, because the metrics worth exporting differ per role. Baking the name in
	// would have excluded the product with the most roles.
	for _, name := range []string{"namenode.yaml", "datanode.yaml", "journalnode.yaml"} {
		got := constant.JMXJavaAgentOpt(8183, name)
		want := "-javaagent:/kubedoop/jmx/jmx_prometheus_javaagent.jar=8183:/kubedoop/jmx/" + name
		if got != want {
			t.Errorf("JMXJavaAgentOpt(8183, %q) = %q, want %q", name, got, want)
		}
	}
}

func TestKubedoopJmxAgentJar(t *testing.T) {
	// The unversioned symlink the images provide, so a product never names a version.
	if constant.KubedoopJmxAgentJar != "/kubedoop/jmx/jmx_prometheus_javaagent.jar" {
		t.Errorf("KubedoopJmxAgentJar = %q", constant.KubedoopJmxAgentJar)
	}
	if got := constant.KubedoopJmxDir; got != "/kubedoop/jmx/" {
		t.Errorf("KubedoopJmxDir = %q", got)
	}
}
