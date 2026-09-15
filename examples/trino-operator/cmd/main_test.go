package main

import (
	"strings"
	"testing"
	"time"
)

func TestMissingHelperImagesFailBeforeClusterConnection(t *testing.T) {
	for _, images := range [][2]string{{"", "vector"}, {"materializer", ""}, {"", ""}} {
		err := start(images[0], images[1], "", "0", "0", "", false, time.Second)
		if err == nil || !strings.Contains(err.Error(), "--materializer-image and --vector-image are required") {
			t.Fatalf("missing helper image did not fail at startup: %v", err)
		}
	}
}
