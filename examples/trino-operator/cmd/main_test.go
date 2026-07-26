/*
Copyright 2026 ZNCDataDev.

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

package main

import (
	"crypto/tls"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/util/cert"
)

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Manager Suite")
}

// manifestFlagPattern matches a command-line argument in a container args list
// ("- --leader-elect") or in a JSON 6902 patch ("value: --metrics-cert-path=..."), so prose in
// comments is not mistaken for a flag.
var manifestFlagPattern = regexp.MustCompile(`(?m)^\s*(?:-|value:)\s+--([a-zA-Z0-9-]+)`)

// manifestFlags collects the flags the deployment manifests pass to the manager container.
func manifestFlags() []string {
	var flags []string
	for _, dir := range []string{
		filepath.Join("..", "config", "manager"),
		filepath.Join("..", "config", "default"),
	} {
		entries, err := os.ReadDir(dir)
		Expect(err).NotTo(HaveOccurred())
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			Expect(err).NotTo(HaveOccurred())
			for _, match := range manifestFlagPattern.FindAllStringSubmatch(string(content), -1) {
				flags = append(flags, match[1])
			}
		}
	}
	return flags
}

// writeSelfSignedCert drops a tls.crt/tls.key pair into dir; certwatcher.New loads the keypair
// eagerly, so the files have to be valid.
func writeSelfSignedCert(dir string) {
	certPEM, keyPEM, err := cert.GenerateSelfSignedCertKey("localhost", nil, nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0o600)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0o600)).To(Succeed())
}

var _ = Describe("Manager flags", func() {
	It("declares every flag the deployment manifests pass", func() {
		fs := flag.NewFlagSet("manager", flag.ContinueOnError)
		registerManagerFlags(fs)

		flags := manifestFlags()
		// The manifests always pass at least --leader-elect, --health-probe-bind-address,
		// --metrics-bind-address and --webhook-cert-path; fewer means the scan broke.
		Expect(len(flags)).To(BeNumerically(">=", 4))

		for _, name := range flags {
			Expect(fs.Lookup(name)).NotTo(BeNil(), "flag --%s is passed by the deployment but not declared", name)
		}
	})
})

var _ = Describe("buildManagerOptions", func() {
	It("serves metrics on the requested address with authn/authz", func() {
		opts, watchers, err := buildManagerOptions(&managerFlags{
			metricsAddr:   ":8443",
			secureMetrics: true,
			probeAddr:     ":8081",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(watchers).To(BeEmpty())

		Expect(opts.Metrics.BindAddress).To(Equal(":8443"))
		Expect(opts.Metrics.SecureServing).To(BeTrue())
		Expect(opts.Metrics.FilterProvider).NotTo(BeNil())
		Expect(opts.HealthProbeBindAddress).To(Equal(":8081"))
		Expect(opts.WebhookServer).NotTo(BeNil())
	})

	It("leaves the metrics endpoint unfiltered when it is served over plain HTTP", func() {
		opts, _, err := buildManagerOptions(&managerFlags{
			metricsAddr:   ":8080",
			secureMetrics: false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Metrics.SecureServing).To(BeFalse())
		Expect(opts.Metrics.FilterProvider).To(BeNil())
	})

	It("watches the mounted certificates so rotation does not need a restart", func() {
		certDir := GinkgoT().TempDir()
		writeSelfSignedCert(certDir)

		opts, watchers, err := buildManagerOptions(&managerFlags{
			metricsAddr:     ":8443",
			secureMetrics:   true,
			metricsCertPath: certDir,
			metricsCertName: "tls.crt",
			metricsCertKey:  "tls.key",
			webhookCertPath: certDir,
			webhookCertName: "tls.crt",
			webhookCertKey:  "tls.key",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(watchers).To(HaveLen(2))
		Expect(opts.Metrics.TLSOpts).NotTo(BeEmpty())
	})

	It("disables HTTP/2 unless it is explicitly enabled", func() {
		opts, _, err := buildManagerOptions(&managerFlags{metricsAddr: "0"})
		Expect(err).NotTo(HaveOccurred())

		cfg := &tls.Config{}
		for _, apply := range opts.Metrics.TLSOpts {
			apply(cfg)
		}
		Expect(cfg.NextProtos).To(Equal([]string{"http/1.1"}))

		opts, _, err = buildManagerOptions(&managerFlags{metricsAddr: "0", enableHTTP2: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Metrics.TLSOpts).To(BeEmpty())
	})
})
