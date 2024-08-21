package util

import (
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/constants"
)

// Subdirectory of the log directory containing files to control the Vector instance
const (
	VectorLogDir = "_vector"

	// File to signal that Vector should be gracefully shut down
	ShutdownFile = "shutdown"
)

const (
	InvokePrepareSignalHandlers = "prepare_signal_handlers"
	InvokeWaitForTermination    = "wait_for_termination $!"
)

const CommonBashTrapFunctions = `prepare_signal_handlers()
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
	`

// Use this command to remove the shutdown file (if it exists) created by [`create_vector_shutdown_file_command`].
// You should execute this command before starting your application.
func RemoveVectorShutdownFileCommand() string {
	return fmt.Sprintf("rm -f %s/%s/%s", constants.KubedoopLogDir, VectorLogDir, ShutdownFile)
}

// Command to create a shutdown file for the vector container.
// Please delete it before starting your application using `RemoveVectorShutdownFileCommand` .
func CreateVectorShutdownFileCommand() string {
	return fmt.Sprintf("mkdir -p %s/%s && touch %s/%s/%s", constants.KubedoopLogDir, VectorLogDir, constants.KubedoopLogDir, VectorLogDir, ShutdownFile)
}

// ExportPodAddress fetch the pod address from the default-address directory and export it as POD_ADDRESS
// the listener was provided by listener operator
func ExportPodAddress() string {
	return fmt.Sprintf(`if [[ -d %s ]]; then
  export POD_ADDRESS=$(cat %s/default-address/address)
  for i in %s/default-address/ports/*; do
      export $(basename $i | tr a-z A-Z)_PORT="$(cat $i)"
  done
fi`, constants.KubedoopListenerDir, constants.KubedoopListenerDir, constants.KubedoopListenerDir)
}
