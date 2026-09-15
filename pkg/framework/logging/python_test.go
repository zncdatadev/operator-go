package logging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

func TestPythonNativeConsumerThresholds(t *testing.T) {
	for _, fileEnabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "file-enabled", false: "file-off"}[fileEnabled], func(t *testing.T) {
			directory := t.TempDir()
			filename := filepath.Join(directory, "server.log")
			fileLevel := pythonDebugLevel
			if !fileEnabled {
				fileLevel = pythonOffLevel
			}
			content, err := Python(framework.ContainerLogging{
				Console: framework.Logger{Level: "WARN"}, File: framework.Logger{Level: fileLevel},
				Loggers: map[string]framework.Logger{"ROOT": {Level: "INFO"}, "product": {Level: pythonDebugLevel}}}, filename)
			if err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(directory, "logging.json")
			if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			// This executes the generated document in the actual native consumer,
			// not a second Go implementation of Python's filtering behavior.
			code := `import json,logging,logging.config,sys
logging.config.dictConfig(json.load(open(sys.argv[1])))
logging.getLogger('product').debug('product-debug-marker')
logging.getLogger('product').warning('product-warning-marker')
logging.getLogger('other').debug('other-debug-hidden')
logging.getLogger('other').info('other-info-marker')
logging.shutdown()
`
			output, err := exec.Command("python3", "-c", code, config).CombinedOutput()
			if err != nil {
				t.Fatalf("native Python logging failed: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), "product-warning-marker") || strings.Contains(string(output), "debug-marker") ||
				strings.Contains(string(output), "other-info-marker") {
				t.Fatalf("console did not consume its own threshold: %s", output)
			}
			data, err := os.ReadFile(filename)
			if !fileEnabled {
				if !os.IsNotExist(err) {
					t.Fatalf("OFF file sink created a file: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"product-debug-marker", "product-warning-marker", "other-info-marker"} {
				if !strings.Contains(string(data), marker) {
					t.Fatalf("file did not contain %q: %s", marker, data)
				}
			}
			if strings.Contains(string(data), "other-debug-hidden") {
				t.Fatalf("root logger threshold ignored: %s", data)
			}
		})
	}
}
