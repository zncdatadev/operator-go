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

package handlers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/handlers"
	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
)

var _ = Describe("AddLogging", func() {
	It("renders a log4j2 file from the merged CRD logging spec", func() {
		buildCtx := &reconciler.RoleGroupBuildContext{
			MergedConfig: &config.MergedConfig{
				Logging: &v1alpha1.LoggingSpec{
					Containers: map[string]v1alpha1.LoggingConfigSpec{
						"trino": {
							Loggers: map[string]*v1alpha1.LogLevelSpec{
								"ROOT":     {Level: "WARN"},
								"io.trino": {Level: "DEBUG"},
							},
						},
					},
				},
			},
		}
		data := map[string]string{}
		Expect(handlers.AddLogging(buildCtx, data)).To(Succeed())
		Expect(data).To(HaveKey("log4j2.properties"))
		Expect(data["log4j2.properties"]).To(ContainSubstring("rootLogger.level=WARN"))
		Expect(data["log4j2.properties"]).To(ContainSubstring("logger.io_trino.level=DEBUG"))
	})

	It("falls back to defaults when no logging is configured", func() {
		buildCtx := &reconciler.RoleGroupBuildContext{MergedConfig: &config.MergedConfig{}}
		data := map[string]string{}
		Expect(handlers.AddLogging(buildCtx, data)).To(Succeed())
		Expect(data["log4j2.properties"]).To(ContainSubstring("rootLogger.level=INFO"))
	})
})
