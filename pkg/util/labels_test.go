package util_test

import (
	"testing"

	"github.com/zncdatadev/operator-go/pkg/util"
)

func TestMergeStringMaps(t *testing.T) {
	map1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	map2 := map[string]string{
		"key3": "value3",
		"key4": "value4",
	}
	expected := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
	}

	result := util.MergeStringMaps(map1, map2)

	if len(result) != len(expected) {
		t.Errorf("Expected length of merged map to be %d, but got %d", len(expected), len(result))
	}

	for k, v := range expected {
		if result[k] != v {
			t.Errorf("Expected merged map to have key-value pair (%s: %s), but got (%s: %s)", k, v, k, result[k])
		}
	}
}

func TestMergeStringMaps_Overtake(t *testing.T) {
	map1 := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	map2 := map[string]string{
		"key2": "value3",
		"key4": "value4",
	}
	expected := map[string]string{
		"key1": "value1",
		"key2": "value3",
		"key4": "value4",
	}

	result := util.MergeStringMaps(map1, map2)

	if len(result) != len(expected) {
		t.Errorf("Expected length of merged map to be %d, but got %d", len(expected), len(result))
	}

	for k, v := range expected {
		if result[k] != v {
			t.Errorf("Expected merged map to have key-value pair (%s: %s), but got (%s: %s)", k, v, k, result[k])
		}
	}
}
