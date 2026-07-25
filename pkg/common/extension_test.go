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

package common_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/common"
)

var _ = Describe("BaseExtension", func() {
	It("should report the name it was constructed with", func() {
		Expect(common.NewBaseExtension("my-extension").Name()).To(Equal("my-extension"))
	})

	It("should satisfy the Extension interface when embedded", func() {
		var ext common.Extension = common.NewBaseExtension("embedded")
		Expect(ext.Name()).To(Equal("embedded"))
	})
})

var _ = Describe("ExtensionError", func() {
	It("should name the failing extension in the message", func() {
		err := common.NewExtensionError("my-extension", errors.New("boom"))
		Expect(err.Error()).To(Equal("extension my-extension: boom"))
	})

	It("should unwrap to the underlying error", func() {
		// The hook loops wrap every failure, so a caller matching on its own sentinel — or on a
		// typed API error — must still be able to see through the wrapper.
		cause := errors.New("boom")
		err := common.NewExtensionError("my-extension", cause)

		Expect(errors.Unwrap(err)).To(BeIdenticalTo(cause))
		Expect(errors.Is(err, cause)).To(BeTrue())
	})
})
