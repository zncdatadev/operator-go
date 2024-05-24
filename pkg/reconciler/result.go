package reconciler

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

type Result struct {
	requeue      bool
	requeueAfter time.Duration
	error        error
}

func NewResult(requeue bool, requeueAfter time.Duration, err error) Result {
	return Result{requeue: requeue, requeueAfter: requeueAfter, error: err}
}

func (r *Result) RequeueOrNot() bool {
	if r.requeue || r.requeueAfter > 0 {
		return true
	}
	return false
}

func (r *Result) Result() (ctrl.Result, error) {
	if r.RequeueOrNot() {
		return ctrl.Result{Requeue: r.requeue, RequeueAfter: r.requeueAfter}, r.error
	}
	return ctrl.Result{}, r.error
}
