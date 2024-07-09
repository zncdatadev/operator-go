package util

import (
	corev1 "k8s.io/api/core/v1"
)

var DefaultRepository = "qury.io/zncdatadev"

// TODO: add semver validation for version fields

// Image represents a container image
// Required fields:
// - ProductName
// - StackVersion
// - ProductVersion
//
// If Custom is set, it will be used as the image tag,
// so the Custom field must be a valid image tag, eg. "myrepo/myimage:latest"
type Image struct {
	Custom         string
	Repository     string
	ProductName    string
	StackVersion   string
	ProductVersion string
	PullPolicy     corev1.PullPolicy
	PullSecretName string
}

type ImageOption func(*ImageOptions)

type ImageOptions struct {
	Custom         string
	Repository     string
	PullPolicy     corev1.PullPolicy
	PullSecretName string
}

// NewImage creates a new Image object
//
// Example:
//
//	image := util.NewImage(
//		"myproduct",
//		"1.0",
//		"1.0.0",
//	)
//
//	image := util.NewImage(
//		"myproduct",
//		"1.0",
//		"1.0.0",
//		func (options *util.ImageOptions) {
//			options.Custom = "myrepo/myimage:latest"
//		}
//	)
func NewImage(
	productName string,
	stackVersion string,
	productVersion string,
	opts ...ImageOption,
) *Image {

	options := &ImageOptions{}

	for _, opt := range opts {
		opt(options)
	}

	return &Image{
		Custom:         options.Custom,
		Repository:     options.Repository,
		ProductName:    productName,
		StackVersion:   stackVersion,
		ProductVersion: productVersion,
		PullPolicy:     options.PullPolicy,
		PullSecretName: options.PullSecretName,
	}
}

func (i *Image) GetImageTag() string {
	if i.Custom != "" {
		return i.Custom
	}

	if i.ProductVersion == "" {
		panic("ProductVersion is required")
	}

	if i.StackVersion == "" {
		panic("StackVersion is required")
	}

	if i.Repository == "" {
		i.Repository = DefaultRepository
	}

	return i.Repository + "/" + i.ProductName + ":" + i.ProductVersion + "-" + "stack" + i.StackVersion
}

func (i *Image) String() string {
	return i.GetImageTag()
}
