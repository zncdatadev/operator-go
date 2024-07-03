package reconciler

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

type Result struct {
	Requeue      bool
	RequeueAfter time.Duration
	Error        error
}

func NewResult(requeue bool, requeueAfter time.Duration, err error) *Result {
	return &Result{Requeue: requeue, RequeueAfter: requeueAfter, Error: err}
}

func (r *Result) RequeueOrNot() bool {
	if r.Requeue || r.RequeueAfter > 0 {
		return true
	}
	return false
}

func (r *Result) CtrlResult() (ctrl.Result, error) {
	if r.RequeueOrNot() {
		return ctrl.Result{Requeue: r.Requeue, RequeueAfter: r.RequeueAfter}, r.Error
	}
	return ctrl.Result{}, r.Error
}
