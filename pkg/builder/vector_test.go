package builder_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zncdatadev/operator-go/pkg/builder"
)

func TestLogProviderCommandArgs(t *testing.T) {
	entrypointScript := `
echo 'Hello, World!'
foo() {
    echo 'foo'
}
`

	expectedArgs := []string{
		`
prepare_signal_handlers()
{
    unset term_child_pid
    unset term_kill_needed
    trap 'handle_term_signal' TERM
}

handle_term_signal()
{
    if [ "${term_child_pid}" ]; then
        kill -TERM "${term_child_pid}" 2>/dev/null
    else
        term_kill_needed="yes"
    fi
}

wait_for_termination()
{
    set +e
    term_child_pid=$1
    if [[ -v term_kill_needed ]]; then
        kill -TERM "${term_child_pid}" 2>/dev/null
    fi
    wait ${term_child_pid} 2>/dev/null
    trap - TERM
    wait ${term_child_pid} 2>/dev/null
    set -e
}

rm -f /zncdata/log/_vector/shutdown
prepare_signal_handlers


echo 'Hello, World!'
foo() {
    echo 'foo'
}


wait_for_termination $!
mkdir -p /zncdata/log/_vector && touch /zncdata/log/_vector/shutdown
`,
	}

	args, err := builder.LogProviderCommand(entrypointScript)
	assert.NoError(t, err)
	assert.Equal(t, expectedArgs, args)
}

func TestVectorYamlFormatter(t *testing.T) {
	actualYaml, err := builder.ParseVectorYaml(map[string]interface{}{
		"LogDir":                  "zncdata/log",
		"Namespace":               "default",
		"Cluster":                 "simple-trino",
		"Role":                    "coordinator",
		"GroupName":               "default",
		"VectorAggregatorAddress": "localhost:8080",
	})
	expectYaml := `api:
  enabled: true
data_dir: /zncdata/vector/var
log_schema:
  host_key: "pod"
sources:
  files_airlift:
    type: "file"
    include:
      - "zncdata/log/*/*.airlift.json"
transforms:
  processed_files_airlift:
    inputs:
      - files_airlift
    type: remap
    source: |
      parsed_event = parse_json!(string!(.message))
      .message = join!(compact([parsed_event.message, parsed_event.stackTrace]), "\n")
      .timestamp = parse_timestamp!(parsed_event.timestamp, "%Y-%m-%dT%H:%M:%S.%fZ")
      .logger = parsed_event.logger
      .level = parsed_event.level
      .thread = parsed_event.thread
  extended_logs_files:
    inputs:
      - processed_files_*
    type: remap
    source: |
      . |= parse_regex!(.file, r'^/zncdata/log/(?P<container>.*?)/(?P<file>.*?)$')
      del(.source_type)
  extended_logs:
    inputs:
      - extended_logs_*
    type: remap
    source: |
      .namespace = "default"
      .cluster = "simple-trino"
      .role = "coordinator"
      .roleGroup = "default"
sinks:
  aggregator:
    inputs:
      - extended_logs
    type: vector
    address: "localhost:8080"
`
	assert.Equal(t, expectYaml, actualYaml)
	assert.NoError(t, err)
}
