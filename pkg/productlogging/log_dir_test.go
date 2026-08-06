/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package productlogging_test

import (
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/productlogging"
)

func TestLogDirFor_DefaultsToContainer(t *testing.T) {
	got := productlogging.LogDirFor(productlogging.ContainerLogging{Container: "Node"})
	// Byte-identical to what ContainerLogDir has always produced: an unset LogDirName must not
	// move a single existing product's log path.
	if want := productlogging.ContainerLogDir("Node"); got != want {
		t.Fatalf("LogDirFor = %q, want %q", got, want)
	}
	if got != "/kubedoop/log/node" {
		t.Fatalf("LogDirFor = %q, want /kubedoop/log/node", got)
	}
}

func TestLogDirFor_HonorsOverride(t *testing.T) {
	decl := productlogging.ContainerLogging{Container: "node", LogDirName: "Superset"}
	if got, want := productlogging.LogDirFor(decl), "/kubedoop/log/superset"; got != want {
		t.Fatalf("LogDirFor = %q, want %q", got, want)
	}
	// The segment IS the Vector event's container tag — the collector extracts it from the path
	// and from nothing else — so this is the whole feature in one assertion.
	if got, want := productlogging.LogDirSegment(decl), "superset"; got != want {
		t.Fatalf("LogDirSegment = %q, want %q", got, want)
	}
}

func TestRenderConfigFile_AppenderFollowsLogDirName(t *testing.T) {
	decl := productlogging.ContainerLogging{
		Container:  "node",
		Framework:  productlogging.LoggingFrameworkLogback,
		LogDirName: "superset",
	}
	_, content, err := productlogging.RenderConfigFile(nil, decl, true)
	if err != nil {
		t.Fatalf("RenderConfigFile() error = %v", err)
	}
	if !strings.Contains(content, "/kubedoop/log/superset/") {
		t.Errorf("appender must write under the overridden directory; got:\n%s", content)
	}
	if strings.Contains(content, "/kubedoop/log/node/") {
		t.Errorf("appender must not write under the container-derived directory; got:\n%s", content)
	}
	// The log FILE name still follows the pod container, which is what keeps two producers
	// sharing a directory from resolving to one file.
	if !strings.Contains(content, "node.") {
		t.Errorf("log file base name must still follow the container name; got:\n%s", content)
	}
}

func TestRenderConfigFile_RejectsUnusableLogDirName(t *testing.T) {
	// Each of these reaches an unquoted /bin/sh -c in the Vector sidecar's command AND one path
	// segment the collector's non-greedy regex splits on.
	for _, bad := range []string{
		"a b",               // splits the mkdir into two arguments
		"x; touch /tmp/pwn", // command injection
		"$(id)",             // command substitution
		"a/b",               // silently truncates the tag at the separator
		"..",                // escapes the log root
		".",                 // collapses to the log root, where the glob no longer matches
		"-lead",             // not an RFC 1123 label
		"UPPER_SNAKE",       // underscore is not a label character
		strings.Repeat("a", 64),
	} {
		decl := productlogging.ContainerLogging{
			Container:  "node",
			Framework:  productlogging.LoggingFrameworkLogback,
			LogDirName: bad,
		}
		if _, _, err := productlogging.RenderConfigFile(nil, decl, true); err == nil {
			t.Errorf("RenderConfigFile(logDirName=%q) = nil error, want a rejection", bad)
		}
		// Also rejected with the appender OFF: an unusable declaration is a mistake whether or
		// not Vector happens to be wired this cycle.
		if _, _, err := productlogging.RenderConfigFile(nil, decl, false); err == nil {
			t.Errorf("RenderConfigFile(logDirName=%q, withFileAppender=false) = nil error, want a rejection", bad)
		}
	}
}

func TestRenderConfigFile_AcceptsLegalLogDirName(t *testing.T) {
	for _, ok := range []string{"superset", "superset-web", "s3", "a"} {
		decl := productlogging.ContainerLogging{
			Container:  "node",
			Framework:  productlogging.LoggingFrameworkLogback,
			LogDirName: ok,
		}
		if _, _, err := productlogging.RenderConfigFile(nil, decl, true); err != nil {
			t.Errorf("RenderConfigFile(logDirName=%q) error = %v, want success", ok, err)
		}
	}
}

func TestValidateProducers_RejectsCollidingLogFiles(t *testing.T) {
	// Two producers sharing a directory AND pinning the same file name: two appenders with
	// independent rotation policies on one file in one emptyDir, losing entries with nothing to
	// show for it. This is the only reachable collision, because the default file base name
	// follows the pod container name, which is unique within a pod.
	decls := []productlogging.ContainerLogging{
		{Container: "web", Framework: productlogging.LoggingFrameworkLogback, LogDirName: "superset", LogFileName: "app.log4j.xml"},
		{Container: "worker", Framework: productlogging.LoggingFrameworkLogback, LogDirName: "superset", LogFileName: "app.log4j.xml"},
	}
	err := productlogging.ValidateProducers(decls)
	if err == nil {
		t.Fatal("ValidateProducers() = nil, want a collision rejection")
	}
	for _, want := range []string{"web", "worker", "/kubedoop/log/superset/app.log4j.xml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q", err, want)
		}
	}
}

func TestValidateProducers_AllowsSharedDirectoryWithDistinctFiles(t *testing.T) {
	// Sharing a directory is coherent and must stay legal: one product tag, several containers,
	// distinct files. The default file names differ because they follow the container names.
	decls := []productlogging.ContainerLogging{
		{Container: "web", Framework: productlogging.LoggingFrameworkLogback, LogDirName: "superset"},
		{Container: "worker", Framework: productlogging.LoggingFrameworkLogback, LogDirName: "superset"},
	}
	if err := productlogging.ValidateProducers(decls); err != nil {
		t.Fatalf("ValidateProducers() = %v, want success", err)
	}
}

func TestValidateProducers_DefaultDeclarationsAlwaysPass(t *testing.T) {
	// Every declaration that renders today must keep rendering: the container-derived default is
	// deliberately not put through the label check, since it is already constrained upstream and
	// may legally carry an uppercase letter (ContainerLogDir lowercases it).
	decls := []productlogging.ContainerLogging{
		{Container: "Node", Framework: productlogging.LoggingFrameworkLogback},
		{Container: "vector", Framework: productlogging.LoggingFrameworkPython},
	}
	if err := productlogging.ValidateProducers(decls); err != nil {
		t.Fatalf("ValidateProducers() = %v, want success", err)
	}
}
