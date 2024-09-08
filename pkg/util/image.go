package util

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

var DefaultRepository = "quay.io/zncdatadev"
var DefaultImagePullPolicy = corev1.PullIfNotPresent

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
	PullPolicy      corev1.PullPolicy
	PullSecretName  string
}

type ImageOption func(*ImageOptions)

type ImageOptions struct {
	Custom         string
	Repo           string
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

func (i *Image) GetImageWithTag() (string, error) {
	if i.Custom != "" {
		return i.Custom, nil
	}

	if i.ProductVersion == "" {
		return "", fmt.Errorf("ProductVersion is required in Image")
	}

	if i.PlatformVersion == "" {
		return "", fmt.Errorf("PlatformVersion is required in Image")
	}

	if i.Repo == "" {
		i.Repo = DefaultRepository
	}
	// quay.io/zncdatadev/myproduct:1.0.0-kubedoop1.0
	return fmt.Sprintf("%s/%s:%s-kubedoop%s", i.Repo, i.ProductName, i.ProductVersion, i.PlatformVersion), nil
}

func (i *Image) String() string {
	tag, err := i.GetImageWithTag()
	if err != nil {
		panic(err)
	}
	return tag
}

func (i *Image) GetPullPolicy() corev1.PullPolicy {
	if i.PullPolicy == "" {
		return DefaultImagePullPolicy
	}
	return i.PullPolicy
}
