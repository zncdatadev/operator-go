package util_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/util"
)

func TestMergePodTemplate(t *testing.T) {
	original := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "container1",
					Image: "image1",
				},
			},
		},
	}
	override := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "container2",
					Image: "image2",
				},
			},
		},
	}

	expectedContainers := []corev1.Container{
		{
			Name:  "container2",
			Image: "image2",
		},
		{
			Name:  "container1",
			Image: "image1",
		},
	}

	merged, err := util.MergeObjectWithStrategic(original, override)
	assert.NoError(t, err)

	assert.Equal(t, len(merged.Spec.Containers), 2)
	assert.Equal(t, merged.Spec.Containers, expectedContainers)

}

var _ = Describe("MergeObject", func() {

	It("should handle nil override config", func() {
		original := &commonsv1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"key1": "value1"},
		}

		merged, err := util.MergeObjectWithJson(original, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(merged.EnvOverrides).To(Equal(map[string]string{"key1": "value1"}))
	})

	It("should handle nil original config", func() {
		override := &commonsv1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"key1": "new-value1"},
		}

		merged, err := util.MergeObjectWithJson(nil, override)
		Expect(err).ToNot(HaveOccurred())
		Expect(merged.EnvOverrides).To(Equal(map[string]string{"key1": "new-value1"}))
	})

	It("should merge two configs correctly", func() {
		original := &commonsv1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"key1": "value1", "key2": "value2"},
		}
		override := &commonsv1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"key1": "new-value1"},
		}

		merged, err := util.MergeObjectWithJson(original, override)
		Expect(err).ToNot(HaveOccurred())
		Expect(merged.EnvOverrides).To(Equal(map[string]string{"key1": "new-value1", "key2": "value2"}))
	})

	It("should merge two configs correctly with empty values", func() {
		original := &commonsv1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"key1": "value1", "key2": "value2"},
		}
		override := &commonsv1alpha1.OverridesSpec{
			EnvOverrides: map[string]string{"key1": "new-value1", "key2": ""},
		}

		merged, err := util.MergeObjectWithJson(original, override)
		Expect(err).ToNot(HaveOccurred())
		Expect(merged.EnvOverrides).To(Equal(map[string]string{"key1": "new-value1", "key2": ""}))
	})

	It("should merge two OverrideSpec correctly with complex values", func() {
		original := &commonsv1alpha1.OverridesSpec{
			CliOverrides: []string{"-Dkey1=value1"},
			ConfigOverrides: map[string]map[string]string{
				"core-site.xml": {
					"key1": "value1",
				},
				"mapred-site.xml": {
					"key1": "value1",
				},
			},
			EnvOverrides: map[string]string{"key1": "value1", "key2": "value2"},
		}
		override := &commonsv1alpha1.OverridesSpec{
			CliOverrides: []string{"-Dkey2=value2"},
			ConfigOverrides: map[string]map[string]string{
				"core-site.xml": {
					"key1": "new-value1",
					"key2": "value2",
				},
				"hdfs-site.xml": {
					"key1": "value1",
				},
			},
			EnvOverrides: map[string]string{"key1": "new-value1", "key3": "value3"},
		}

		merged, err := util.MergeObjectWithJson(original, override)
		Expect(err).ToNot(HaveOccurred())
		Expect(merged.CliOverrides).To(Equal([]string{"-Dkey2=value2"}))
		Expect(merged.ConfigOverrides).To(Equal(map[string]map[string]string{
			"core-site.xml": {
				"key1": "new-value1",
				"key2": "value2",
			},
			"mapred-site.xml": {
				"key1": "value1",
			},
			"hdfs-site.xml": {
				"key1": "value1",
			},
		}))
		Expect(merged.EnvOverrides).To(Equal(map[string]string{"key1": "new-value1", "key2": "value2", "key3": "value3"}))
	})

	It("should merge two RoleGroupConfigSpec correctly with complex values", func() {
		original := &commonsv1alpha1.RoleGroupConfigSpec{
			Logging: &commonsv1alpha1.LoggingSpec{
				Containers: map[string]commonsv1alpha1.LoggingConfigSpec{
					"container1": {
						Loggers: map[string]*commonsv1alpha1.LogLevelSpec{
							"logger1": {Level: "INFO"},
						},
					},
				},
			},
			Resources: &commonsv1alpha1.ResourcesSpec{
				CPU: &commonsv1alpha1.CPUResource{
					Max: resource.MustParse("100m"),
				},
			},
		}
		override := &commonsv1alpha1.RoleGroupConfigSpec{
			Logging: &commonsv1alpha1.LoggingSpec{
				Containers: map[string]commonsv1alpha1.LoggingConfigSpec{
					"container1": {
						File: &commonsv1alpha1.LogLevelSpec{Level: "DEBUG"},
					},
				},
				EnableVectorAgent: ptr.To(true),
			},
			Resources: &commonsv1alpha1.ResourcesSpec{
				CPU: &commonsv1alpha1.CPUResource{
					Max: resource.MustParse("200m"),
					Min: resource.MustParse("100m"),
				},
				Memory: &commonsv1alpha1.MemoryResource{
					Limit: resource.MustParse("1Gi"),
				},
			},
		}

		merged, err := util.MergeObjectWithJson(original, override)
		Expect(err).ToNot(HaveOccurred())
		Expect(merged.Logging).To(Equal(&commonsv1alpha1.LoggingSpec{
			Containers: map[string]commonsv1alpha1.LoggingConfigSpec{
				"container1": {
					Loggers: map[string]*commonsv1alpha1.LogLevelSpec{
						"logger1": {Level: "INFO"},
					},
					File: &commonsv1alpha1.LogLevelSpec{Level: "DEBUG"},
				},
			},
			EnableVectorAgent: ptr.To(true),
		}))
		Expect(merged.Resources).To(Equal(&commonsv1alpha1.ResourcesSpec{
			CPU: &commonsv1alpha1.CPUResource{
				Max: resource.MustParse("200m"),
				Min: resource.MustParse("100m"),
			},
			Memory: &commonsv1alpha1.MemoryResource{
				Limit: resource.MustParse("1Gi"),
			},
		}))

	})

})
