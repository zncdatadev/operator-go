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

package v1alpha1_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

var _ = Describe("GenericClusterStatus", func() {

	Describe("SetCondition", func() {
		Context("when the condition type is new", func() {
			It("appends it and stamps a LastTransitionTime", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetCondition(metav1.Condition{
					Type:    string(v1alpha1.ConditionAvailable),
					Status:  metav1.ConditionTrue,
					Reason:  v1alpha1.ReasonAvailable,
					Message: "all replicas are available",
				})

				cond := status.GetCondition(v1alpha1.ConditionAvailable)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.LastTransitionTime.IsZero()).To(BeFalse())
			})

			It("honours an explicitly supplied LastTransitionTime", func() {
				stamp := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
				status := &v1alpha1.GenericClusterStatus{}
				status.SetCondition(metav1.Condition{
					Type:               string(v1alpha1.ConditionAvailable),
					Status:             metav1.ConditionTrue,
					Reason:             v1alpha1.ReasonAvailable,
					LastTransitionTime: stamp,
				})

				Expect(status.GetCondition(v1alpha1.ConditionAvailable).LastTransitionTime).To(Equal(stamp))
			})
		})

		Context("when the condition type already exists with the same status", func() {
			It("preserves LastTransitionTime and updates reason and message", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetAvailable(v1alpha1.ReasonCreating, "waiting for replicas")
				first := status.GetCondition(v1alpha1.ConditionAvailable).LastTransitionTime

				status.SetAvailable(v1alpha1.ReasonAvailable, "all replicas are available")

				cond := status.GetCondition(v1alpha1.ConditionAvailable)
				Expect(cond.LastTransitionTime).To(Equal(first))
				Expect(cond.Reason).To(Equal(v1alpha1.ReasonAvailable))
				Expect(cond.Message).To(Equal("all replicas are available"))
			})

			It("leaves the whole status untouched when nothing changed", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetAvailable(v1alpha1.ReasonAvailable, "all replicas are available")
				status.SetReconcileComplete(true, v1alpha1.ReasonReconcileComplete, "done")
				before := status.DeepCopy()

				status.SetAvailable(v1alpha1.ReasonAvailable, "all replicas are available")
				status.SetReconcileComplete(true, v1alpha1.ReasonReconcileComplete, "done")

				Expect(apiequality.Semantic.DeepEqual(before, status)).To(BeTrue())
			})

			It("ignores a newer LastTransitionTime supplied by the caller", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetCondition(metav1.Condition{
					Type:               string(v1alpha1.ConditionDegraded),
					Status:             metav1.ConditionFalse,
					Reason:             v1alpha1.ReasonAvailable,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Hour)),
				})
				first := status.GetCondition(v1alpha1.ConditionDegraded).LastTransitionTime

				status.SetCondition(metav1.Condition{
					Type:               string(v1alpha1.ConditionDegraded),
					Status:             metav1.ConditionFalse,
					Reason:             v1alpha1.ReasonAvailable,
					LastTransitionTime: metav1.Now(),
				})

				Expect(status.GetCondition(v1alpha1.ConditionDegraded).LastTransitionTime).To(Equal(first))
			})
		})

		Context("when the condition status flips", func() {
			It("stamps a new LastTransitionTime", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetCondition(metav1.Condition{
					Type:               string(v1alpha1.ConditionAvailable),
					Status:             metav1.ConditionTrue,
					Reason:             v1alpha1.ReasonAvailable,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Hour)),
				})
				first := status.GetCondition(v1alpha1.ConditionAvailable).LastTransitionTime

				status.SetUnavailable(v1alpha1.ReasonDegraded, "replicas are missing")

				cond := status.GetCondition(v1alpha1.ConditionAvailable)
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.LastTransitionTime.After(first.Time)).To(BeTrue())
			})
		})

		Context("observed generation", func() {
			It("inherits the status observed generation", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetObservedGeneration(7)
				status.SetAvailable(v1alpha1.ReasonAvailable, "all replicas are available")

				Expect(status.GetCondition(v1alpha1.ConditionAvailable).ObservedGeneration).To(Equal(int64(7)))
			})

			It("refreshes on a later reconcile without a status flip", func() {
				status := &v1alpha1.GenericClusterStatus{}
				status.SetObservedGeneration(1)
				status.SetAvailable(v1alpha1.ReasonAvailable, "all replicas are available")
				first := status.GetCondition(v1alpha1.ConditionAvailable).LastTransitionTime

				status.SetObservedGeneration(2)
				status.SetAvailable(v1alpha1.ReasonAvailable, "all replicas are available")

				cond := status.GetCondition(v1alpha1.ConditionAvailable)
				Expect(cond.ObservedGeneration).To(Equal(int64(2)))
				Expect(cond.LastTransitionTime).To(Equal(first))
			})

			It("keeps an explicitly supplied generation", func() {
				status := &v1alpha1.GenericClusterStatus{ObservedGeneration: 3}
				status.SetCondition(metav1.Condition{
					Type:               string(v1alpha1.ConditionProgressing),
					Status:             metav1.ConditionTrue,
					Reason:             v1alpha1.ReasonProgressing,
					ObservedGeneration: 5,
				})

				Expect(status.GetCondition(v1alpha1.ConditionProgressing).ObservedGeneration).To(Equal(int64(5)))
			})
		})

		It("does not duplicate a condition type", func() {
			status := &v1alpha1.GenericClusterStatus{}
			status.SetProgressing(true, v1alpha1.ReasonProgressing, "rolling out")
			status.SetProgressing(false, v1alpha1.ReasonAvailable, "stable")

			Expect(status.Conditions).To(HaveLen(1))
			Expect(status.IsProgressing()).To(BeFalse())
		})
	})
})
