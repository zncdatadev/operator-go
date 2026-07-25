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

// The exec stream error cannot be injected through the exported API — envtest runs no kubelet,
// so no real command ever exits — hence these specs run inside the package. They join the
// Ginkgo suite bootstrapped by suite_test.go.
package util

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// exitStatusError is a stream error carrying the command's own exit status, as
// remotecommand returns for a command that exited non-zero.
type exitStatusError struct {
	status int
}

func (e *exitStatusError) Error() string {
	return fmt.Sprintf("command terminated with exit code %d", e.status)
}

func (e *exitStatusError) ExitStatus() int { return e.status }

var _ = Describe("newExecuteResult", func() {
	It("reports exit code 0 and the captured streams on success", func() {
		result := newExecuteResult("out", "err", nil)
		Expect(result.ExitCode).To(Equal(0))
		Expect(result.Stdout).To(Equal("out"))
		Expect(result.Stderr).To(Equal("err"))
	})

	It("reports the command's own exit status", func() {
		result := newExecuteResult("", "boom", &exitStatusError{status: 42})
		Expect(result.ExitCode).To(Equal(42))
	})

	It("reports the command's own exit status through a wrapped error", func() {
		wrapped := fmt.Errorf("stream failed: %w", &exitStatusError{status: 7})
		Expect(newExecuteResult("", "", wrapped).ExitCode).To(Equal(7))
	})

	It("falls back to 1 for a failure that carries no exit status", func() {
		result := newExecuteResult("", "", errors.New("connection reset"))
		Expect(result.ExitCode).To(Equal(1))
	})
})
