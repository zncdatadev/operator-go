package util_test

import (
	"testing"

	"github.com/zncdatadev/operator-go/pkg/util"
)

func TestGetImageTag(t *testing.T) {
	tests := []struct {
		name  string
		image util.Image
		tag   string
	}{
		{
			name: "Custom tag provided",
			image: util.Image{
				Custom:         "myrepo/myimage:latest",
				ProductName:    "myproduct",
				StackVersion:   "1.0",
				ProductVersion: "1.0.0",
			},
			tag: "myrepo/myimage:latest",
		},
		{
			name: "Default repository and tag",
			image: util.Image{
				ProductName:    "myproduct",
				StackVersion:   "1.0",
				ProductVersion: "1.0.0",
			},
			tag: "qury.io/zncdatadev/myproduct:1.0.0-stack1.0",
		},
		{
			name: "Custom repository",
			image: util.Image{
				Repository:     "example.com",
				ProductName:    "myproduct",
				StackVersion:   "1.0",
				ProductVersion: "1.0.0",
			},
			tag: "example.com/myproduct:1.0.0-stack1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageTag := tt.image.GetImageWithTag()
			if imageTag != tt.tag {
				t.Errorf("Expected tag %s, but got %s", tt.tag, imageTag)
			}
		})
	}
}
