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
