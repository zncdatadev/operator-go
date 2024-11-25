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

package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

rm -f /kubedoop/log/_vector/shutdown
prepare_signal_handlers


echo 'Hello, World!'
foo() {
    echo 'foo'
}


wait_for_termination $!
mkdir -p /kubedoop/log/_vector && touch /kubedoop/log/_vector/shutdown
`,
	}

	args, err := LogProviderCommand(entrypointScript)
	assert.NoError(t, err)
	assert.Equal(t, expectedArgs, args)
}

func TestVectorCommandArgs(t *testing.T) {
	expectedArgs := []string{
		`
# Vector will ignore SIGTERM (as PID != 1) and must be shut down by writing a shutdown trigger file
vector --config /kubedoop/config/vector.yaml & vector_pid=$!
if [ ! -f /kubedoop/log/_vector/shutdown ]; then
    mkdir -p /kubedoop/log/_vector
    inotifywait -qq --event create /kubedoop/log/_vector
fi

sleep 1

kill $vector_pid
`,
	}

	args := VectorCommandArgs()
	assert.Equal(t, expectedArgs, args)
}
