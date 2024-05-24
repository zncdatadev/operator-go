package util

import (
	corev1 "k8s.io/api/core/v1"
)

type Image struct {
	Custom         string            `json:"custom,omitempty"`
	Repo           string            `json:"repository,omitempty"`
	KDSVersion     string            `json:"kdsVersion,omitempty"`
	ProductVersion string            `json:"productVersion,omitempty"`
	PullPolicy     corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

func (i *Image) GetImageTag() string {
	if i.Custom != "" {
		return i.Custom
	}
	return i.Repo + ":" + i.ProductVersion + "-" + i.KDSVersion
}

func (i *Image) String() string {
	return i.GetImageTag()
}

func (i *Image) GetPullPolicy() corev1.PullPolicy {
	return i.PullPolicy
}
