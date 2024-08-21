package util

import (
	corev1 "k8s.io/api/core/v1"
)

var DefaultRepository = "quay.io/zncdatadev"

// TODO: add semver validation for version fields

// Image represents a container image
// Required fields:
//   - ProductName
//   - PlatformVersion
//   - ProductVersion
//
// If Custom is set, it will be used as the image tag,
// so the Custom field must be a valid image tag, eg. "my.repo.company.org/namespace/image:tag"
type Image struct {
	Custom          string
	Repo            string
	ProductName     string
	PlatformVersion string
	ProductVersion  string
	PullPolicy      *corev1.PullPolicy
	PullSecretName  string
}

type ImageOption func(*ImageOptions)

type ImageOptions struct {
	Custom         string
	Repo           string
	PullPolicy     *corev1.PullPolicy
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
	platformVersion string,
	productVersion string,
	opts ...ImageOption,
) *Image {

	options := &ImageOptions{}

	for _, opt := range opts {
		opt(options)
	}

	return &Image{
		Custom:          options.Custom,
		Repo:            options.Repo,
		ProductName:     productName,
		PlatformVersion: platformVersion,
		ProductVersion:  productVersion,
		PullPolicy:      options.PullPolicy,
		PullSecretName:  options.PullSecretName,
	}
}

func (i *Image) GetImageWithTag() string {
	if i.Custom != "" {
		return i.Custom
	}

	if i.ProductVersion == "" {
		panic("ProductVersion is required")
	}

	if i.PlatformVersion == "" {
		panic("PlatformVersion is required")
	}

	if i.Repo == "" {
		i.Repo = DefaultRepository
	}

	return i.Repo + "/" + i.ProductName + ":" + i.ProductVersion + "-" + "kubedoop" + i.PlatformVersion
}

func (i *Image) String() string {
	return i.GetImageWithTag()
}

func (i *Image) GetPullPolicy() *corev1.PullPolicy {
	if i.PullPolicy == nil {
		return func() *corev1.PullPolicy { p := corev1.PullIfNotPresent; return &p }()
	}
	return i.PullPolicy
}
