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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/common"
)

var _ = Describe("RequeueAfterError", func() {
	It("clamps a non-positive delay, which would otherwise never requeue at all", func() {
		// controller-runtime treats RequeueAfter as a switch: `case result.RequeueAfter > 0` falls
		// through to a branch that only Forgets the item. A product computing
		// deadline.Sub(time.Now()) therefore wedges the cluster the moment the deadline passes —
		// the wait is reported, no error is returned, and nothing wakes the controller again.
		Expect(common.NewRequeueAfterError(0, "R", "m").After).To(Equal(common.DefaultRequeueAfter))
		Expect(common.NewRequeueAfterError(-time.Hour, "R", "m").After).To(Equal(common.DefaultRequeueAfter))
	})

	It("clamps a sub-second delay away from a hot loop", func() {
		Expect(common.NewRequeueAfterError(time.Nanosecond, "R", "m").After).To(Equal(common.MinRequeueAfter))
		Expect(common.NewRequeueAfterError(30*time.Second, "R", "m").After).To(Equal(30 * time.Second))
	})

	It("is found through a wrapping chain", func() {
		wrapped := fmt.Errorf("outer: %w", common.NewRequeueAfterError(time.Minute, "R", "m"))
		Expect(common.IsRequeueAfterError(wrapped)).To(BeTrue())
		Expect(common.IsRequeueAfterError(errors.New("plain"))).To(BeFalse())
	})
})

var _ = Describe("WaitingErrors", func() {
	wait := func(d time.Duration) error { return common.NewRequeueAfterError(d, "R", "m") }

	It("reports a lone wait, with its delay", func() {
		waitErr, waiting := common.WaitingErrors(wait(30 * time.Second))
		Expect(waiting).To(BeTrue())
		Expect(waitErr.After).To(Equal(30 * time.Second))
	})

	It("refuses an aggregate that also holds a genuine failure", func() {
		// The decisive rule. The framework joins per-role and per-extension errors, so a tree can
		// hold a wait next to a real failure — and a plain errors.As would find the wait and
		// suppress the failure's Degraded report entirely.
		joined := errors.Join(wait(time.Minute), errors.New("unknown field \"nodeAffinty\""))
		_, waiting := common.WaitingErrors(joined)
		Expect(waiting).To(BeFalse())
	})

	It("takes the SHORTEST delay across a join, not the first one found", func() {
		// errors.As returns the first match depth-first, so a ten-minute wait on one branch would
		// hide a five-second wait on another and the cluster would sit idle for the difference.
		joined := errors.Join(
			common.NewRequeueAfterError(10*time.Minute, "Slow", "slow"),
			common.NewRequeueAfterError(5*time.Second, "Fast", "fast"),
			common.NewRequeueAfterError(time.Hour, "Slowest", "slowest"),
		)
		waitErr, waiting := common.WaitingErrors(joined)
		Expect(waiting).To(BeTrue())
		Expect(waitErr.After).To(Equal(5 * time.Second))
		// The reason must belong to the SAME wait as the delay: reporting "Slow" while waiting five
		// seconds describes a wait that is not the one about to resolve.
		Expect(waitErr.Reason).To(Equal("Fast"))
	})

	It("walks nested joins and wrapping chains", func() {
		nested := errors.Join(
			fmt.Errorf("role a: %w", wait(time.Minute)),
			errors.Join(wait(20*time.Second), wait(time.Hour)),
		)
		waitErr, waiting := common.WaitingErrors(nested)
		Expect(waiting).To(BeTrue())
		Expect(waitErr.After).To(Equal(20 * time.Second))
	})

	It("treats nil and an opaque wrapper as not waiting", func() {
		_, waiting := common.WaitingErrors(nil)
		Expect(waiting).To(BeFalse())

		// A wrapper whose cause cannot be inspected must count as a failure: assuming it is a wait
		// would suppress Degraded for an error nobody can see inside.
		_, waiting = common.WaitingErrors(errors.New("opaque"))
		Expect(waiting).To(BeFalse())
	})
})
