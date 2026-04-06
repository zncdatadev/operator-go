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

package vector

const vectorConfigTemplate = `---
api:
  enabled: true
  address: "127.0.0.1:{{APIPort}}"

sources:
  files_stdout:
    type: "file"
    include:
      - "{{.LogDir}}/*.stdout.log"
    read_from: "beginning"
    line_delimiter: "\n"

  files_stderr:
    type: "file"
    include:
      - "{{.LogDir}}/*.stderr.log"
    read_from: "beginning"
    line_delimiter: "\n"

  files_log4j:
    type: "file"
    include:
      - "{{.LogDir}}/*.log4j.xml"
    read_from: "beginning"
    line_delimiter: "\n"
    multiline:
      mode: "halt_before"
      start_pattern: "^<log4j:event"
      condition_pattern: "^<log4j:event"
      timeout_ms: 10000

  files_log4j2:
    type: "file"
    include:
      - "{{.LogDir}}/*.log4j2.xml"
    read_from: "beginning"
    line_delimiter: "\n"
    multiline:
      mode: "halt_before"
      start_pattern: "^<Event"
      condition_pattern: "^<Event"
      timeout_ms: 10000

  files_py:
    type: "file"
    include:
      - "{{.LogDir}}/*.py.json"
    read_from: "beginning"
    line_delimiter: "\n"

  files_airlift:
    type: "file"
    include:
      - "{{.LogDir}}/*.airlift.json"
    read_from: "beginning"
    line_delimiter: "\n"

  vector:
    type: "internal_logs"

transforms:
  parse_stdout:
    type: "remap"
    inputs:
      - "files_stdout"
    source: |
      .message = string!(.message) ?? ""
      .tags.log_type = "stdout"

  parse_stderr:
    type: "remap"
    inputs:
      - "files_stderr"
    source: |
      .message = string!(.message) ?? ""
      .tags.log_type = "stderr"

  parse_log4j:
    type: "remap"
    inputs:
      - "files_log4j"
    source: |
      .message = string!(.message) ?? ""
      .tags.log_type = "log4j"

  parse_log4j2:
    type: "remap"
    inputs:
      - "files_log4j2"
    source: |
      .message = string!(.message) ?? ""
      .tags.log_type = "log4j2"

  parse_py:
    type: "remap"
    inputs:
      - "files_py"
    source: |
      .message = string!(.message) ?? ""
      .tags.log_type = "py"

  parse_airlift:
    type: "remap"
    inputs:
      - "files_airlift"
    source: |
      .message = string!(.message) ?? ""
      .tags.log_type = "airlift"

  enrich_metadata:
    type: "remap"
    inputs:
      - "parse_stdout"
      - "parse_stderr"
      - "parse_log4j"
      - "parse_log4j2"
      - "parse_py"
      - "parse_airlift"
      - "vector"
    source: |
      .tags.namespace = "{{.Namespace}}"
      .tags.cluster = "{{.ClusterName}}"
      .tags.role = "{{.RoleName}}"
      .tags.role_group = "{{.RoleGroupName}}"

sinks:
  aggregator:
    type: "vector"
    inputs:
      - "enrich_metadata"
    address: "{{.AggregatorAddress}}"
`
