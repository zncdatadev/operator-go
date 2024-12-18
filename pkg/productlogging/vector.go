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

package productlogging

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgclient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/config"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/util"
)

func MakeVectorYaml(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string,
	cluster string,
	roleName string,
	roleGroupName string,
	vectorAggregatorDiscovery string,
) (string, error) {
	vectorAggregatorDiscoveryURI := vectorAggregatorDiscoveryURI(ctx, client, namespace, vectorAggregatorDiscovery)
	data := map[string]interface{}{
		"LogDir":                  constants.KubedoopLogDir,
		"Namespace":               namespace,
		"Cluster":                 cluster,
		"RoleName":                roleName,
		"RoleGroupName":           roleGroupName,
		"VectorAggregatorAddress": vectorAggregatorDiscoveryURI,
	}
	return parseVectorYaml(data)
}

// ```yaml
// api:
//
//	enabled: true
//	address: 0.0.0.0:8686
//	playground: false
//
// ```
// This config above describes the configuration options for the GraphQL API.
// - `enabled`: Indicates whether the GraphQL API is enabled.
// - `address`: The network address to which the API should bind. If running Vector in a Docker container, ensure it binds to 0.0.0.0 to be accessible outside the container.
// - `playground`: Specifies whether the GraphQL Playground is enabled.
func parseVectorYaml(data map[string]interface{}) (string, error) {
	var tmpl = `api:
	enabled: true
	address: 0.0.0.0:8686
	playground: false
data_dir: /kubedoop/vector/var
log_schema:
	host_key: "pod"
sources:
  vector:
    type: internal_logs

  files_stdout:
    type: file
    include:
      - {{.LogDir}}*/*.stdout.log

  files_stderr:
    type: file
    include:
      - {{.LogDir}}*/*.stderr.log

  files_log4j:
    type: file
    include:
      - {{.LogDir}}*/*.log4j.xml
    line_delimiter: "\r\n"
    multiline:
      mode: halt_before
      start_pattern: ^<log4j:event
      condition_pattern: ^<log4j:event
      timeout_ms: 1000

  files_log4j2:
    type: file
    include:
      - {{.LogDir}}*/*.log4j2.xml
    line_delimiter: "\r\n"

  files_py:
    type: file
    include:
      - {{.LogDir}}*/*.py.json

  files_airlift:
    type: "file"
    include:
      - "{{.LogDir}}*/*.airlift.json"

transforms:
  processed_files_stdout:
    inputs:
      - files_stdout
    type: remap
    source: |
      .logger = "ROOT"
      .level = "INFO"

  processed_files_stderr:
    inputs:
      - files_stderr
    type: remap
    source: |
      .logger = "ROOT"
      .level = "ERROR"

  processed_files_log4j:
    inputs:
      - files_log4j
    type: remap
    source: |
      raw_message = string!(.message)

      .timestamp = now()
      .logger = ""
      .level = "INFO"
      .message = ""
      .errors = []

      # Wrap the event so that the log4j namespace is defined when parsing the event
      wrapped_xml_event = "<root xmlns:log4j=\"http://jakarta.apache.org/log4j/\">" + raw_message + "</root>"
      parsed_event, err = parse_xml(wrapped_xml_event)
      if err != null {{"{{"}}
        error = "XML not parsable: " + err
        .errors = push(.errors, error)
        log(error, level: "warn")
        .message = raw_message
      {{"}}"}} else {{"{{"}}
        root = object!(parsed_event.root)
        if !is_object(root.event) {{"{{"}}
          error = "Parsed event contains no \"event\" tag."
          .errors = push(.errors, error)
          log(error, level: "warn")
          .message = raw_message
        {{"}}"}} else {{"{{"}}
          if keys(root) != ["event"] {{"{{"}}
            .errors = push(.errors, "Parsed event contains multiple tags: " + join!(keys(root), ", "))
          {{"}}"}}
          event = object!(root.event)

          epoch_milliseconds, err = to_int(event.@timestamp)
          if err == null && epoch_milliseconds != 0 {{"{{"}}
            converted_timestamp, err = from_unix_timestamp(epoch_milliseconds, "milliseconds")
            if err == null {{"{{"}}
              .timestamp = converted_timestamp
            {{"}}"}} else {{"{{"}}
              .errors = push(.errors, "Time not parsable, using current time instead: " + err)
            {{"}}"}}
          {{"}}"}} else {{"{{"}}
            .errors = push(.errors, "Timestamp not found, using current time instead.")
          {{"}}"}}

          .logger, err = string(event.@logger)
          if err != null || is_empty(.logger) {{"{{"}}
            .errors = push(.errors, "Logger not found.")
          {{"}}"}}

          level, err = string(event.@level)
          if err != null {{"{{"}}
            .errors = push(.errors, "Level not found, using \"" + .level + "\" instead.")
          {{"}}"}} else if !includes(["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"], level) {{"{{"}}
            .errors = push(.errors, "Level \"" + level + "\" unknown, using \"" + .level + "\" instead.")
          {{"}}"}} else {{"{{"}}
            .level = level
          {{"}}"}}

          message, err = string(event.message)
          if err != null || is_empty(message) {{"{{"}}
            .errors = push(.errors, "Message not found.")
          {{"}}"}}
          throwable = string(event.throwable) ?? ""
          .message = join!(compact([message, throwable]), "\n")
        {{"}}"}}
      {{"}}"}}

  processed_files_log4j2:
    inputs:
      - files_log4j2
    type: remap
    source: |
      raw_message = string!(.message)

      .timestamp = now()
      .logger = ""
      .level = "INFO"
      .message = ""
      .errors = []

      event = {{"{{"}}}}
      parsed_event, err = parse_xml(raw_message)
      if err != null {{"{{"}}
        error = "XML not parsable: " + err
        .errors = push(.errors, error)
        log(error, level: "warn")
        .message = raw_message
      {{"}}"}} else {{"{{"}}
        if !is_object(parsed_event.Event) {{"{{"}}
          error = "Parsed event contains no \"Event\" tag."
          .errors = push(.errors, error)
          log(error, level: "warn")
          .message = raw_message
        {{"}}"}} else {{"{{"}}
          event = object!(parsed_event.Event)

          tag_instant_valid = false
          instant, err = object(event.Instant)
          if err == null {{"{{"}}
            epoch_nanoseconds, err = to_int(instant.@epochSecond) * 1_000_000_000 + to_int(instant.@nanoOfSecond)
            if err == null && epoch_nanoseconds != 0 {{"{{"}}
              converted_timestamp, err = from_unix_timestamp(epoch_nanoseconds, "nanoseconds")
              if err == null {{"{{"}}
                .timestamp = converted_timestamp
                tag_instant_valid = true
              {{"}}"}} else {{"{{"}}
                .errors = push(.errors, "Instant invalid, trying property timeMillis instead: " + err)
              {{"}}"}}
            {{"}}"}} else {{"{{"}}
              .errors = push(.errors, "Instant invalid, trying property timeMillis instead: " + err)
            {{"}}"}}
          {{"}}"}}
          if !tag_instant_valid {{"{{"}}
            epoch_milliseconds, err = to_int(event.@timeMillis)
            if err == null && epoch_milliseconds != 0 {{"{{"}}
              converted_timestamp, err = from_unix_timestamp(epoch_milliseconds, "milliseconds")
              if err == null {{"{{"}}
                .timestamp = converted_timestamp
              {{"}}"}} else {{"{{"}}
                .errors = push(.errors, "timeMillis not parsable, using current time instead: " + err)
              {{"}}"}}
            {{"}}"}} else {{"{{"}}
              .errors = push(.errors, "timeMillis not parsable, using current time instead: " + err)
            {{"}}"}}
          {{"}}"}}

          .logger, err = string(event.@loggerName)
          if err != null || is_empty(.logger) {{"{{"}}
            .errors = push(.errors, "Logger not found.")
          {{"}}"}}

          level, err = string(event.@level)
          if err != null {{"{{"}}
            .errors = push(.errors, "Level not found, using \"" + .level + "\" instead.")
          {{"}}"}} else if !includes(["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"], level) {{"{{"}}
            .errors = push(.errors, "Level \"" + level + "\" unknown, using \"" + .level + "\" instead.")
          {{"}}"}} else {{"{{"}}
            .level = level
          {{"}}"}}

          exception = null
          thrown = event.Thrown
          if is_object(thrown) {{"{{"}}
            exception = "Exception"
            thread, err = string(event.@thread)
            if err == null && !is_empty(thread) {{"{{"}}
              exception = exception + " in thread \"" + thread + "\""
            {{"}}"}}
            thrown_name, err = string(thrown.@name)
            if err == null && !is_empty(exception) {{"{{"}}
              exception = exception + " " + thrown_name
            {{"}}"}}
            message = string(thrown.@localizedMessage) ??
              string(thrown.@message) ??
              ""
            if !is_empty(message) {{"{{"}}
              exception = exception + ": " + message
            {{"}}"}}
            stacktrace_items = array(thrown.ExtendedStackTrace.ExtendedStackTraceItem) ?? []
            stacktrace = ""
            for_each(stacktrace_items) -> |_index, value| {{"{{"}}
              stacktrace = stacktrace + "        "
              class = string(value.@class) ?? ""
              method = string(value.@method) ?? ""
              if !is_empty(class) && !is_empty(method) {{"{{"}}
                stacktrace = stacktrace + "at " + class + "." + method
              {{"}}"}}
              file = string(value.@file) ?? ""
              line = string(value.@line) ?? ""
              if !is_empty(file) && !is_empty(line) {{"{{"}}
                stacktrace = stacktrace + "(" + file + ":" + line + ")"
              {{"}}"}}
              exact = to_bool(value.@exact) ?? false
              location = string(value.@location) ?? ""
              version = string(value.@version) ?? ""
              if !is_empty(location) && !is_empty(version) {{"{{"}}
                stacktrace = stacktrace + " "
                if !exact {{"{{"}}
                  stacktrace = stacktrace + "~"
                {{"}}"}}
                stacktrace = stacktrace + "[" + location + ":" + version + "]"
              {{"}}"}}
              stacktrace = stacktrace + "\n"
            {{"}}"}}
            if stacktrace != "" {{"{{"}}
              exception = exception + "\n" + stacktrace
            {{"}}"}}
          {{"}}"}}

          message, err = string(event.Message)
          if err != null || is_empty(message) {{"{{"}}
            message = null
            .errors = push(.errors, "Message not found.")
          {{"}}"}}
          .message = join!(compact([message, exception]), "\n")
        {{"}}"}}
      {{"}}"}}

  processed_files_py:
    inputs:
      - files_py
    type: remap
    source: |
      raw_message = string!(.message)

      .timestamp = now()
      .logger = ""
      .level = "INFO"
      .message = ""
      .errors = []

      parsed_event, err = parse_json(raw_message)
      if err != null {{"{{"}}
        error = "JSON not parsable: " + err
        .errors = push(.errors, error)
        log(error, level: "warn")
        .message = raw_message
      {{"}}"}} else if !is_object(parsed_event) {{"{{"}}
        error = "Parsed event is not a JSON object."
        .errors = push(.errors, error)
        log(error, level: "warn")
        .message = raw_message
      {{"}}"}} else {{"{{"}}
        event = object!(parsed_event)

        asctime, err = string(event.asctime)
        if err == null {{"{{"}}
          parsed_timestamp, err = parse_timestamp(asctime, "%F %T,%3f")
          if err == null {{"{{"}}
            .timestamp = parsed_timestamp
          {{"}}"}} else {{"{{"}}
            .errors = push(.errors, "Timestamp not parsable, using current time instead: "+ err)
          {{"}}"}}
        {{"}}"}} else {{"{{"}}
          .errors = push(.errors, "Timestamp not found, using current time instead.")
        {{"}}"}}

        .logger, err = string(event.name)
        if err != null || is_empty(.logger) {{"{{"}}
          .errors = push(.errors, "Logger not found.")
        {{"}}"}}

        level, err = string(event.levelname)
        if err != null {{"{{"}}
          .errors = push(.errors, "Level not found, using \"" + .level + "\" instead.")
        {{"}}"}} else if level == "DEBUG" {{"{{"}}
          .level = "DEBUG"
        {{"}}"}} else if level == "INFO" {{"{{"}}
          .level = "INFO"
        {{"}}"}} else if level == "WARNING" {{"{{"}}
          .level = "WARN"
        {{"}}"}} else if level == "ERROR" {{"{{"}}
          .level = "ERROR"
        {{"}}"}} else if level == "CRITICAL" {{"{{"}}
          .level = "FATAL"
        {{"}}"}} else {{"{{"}}
          .errors = push(.errors, "Level \"" + level + "\" unknown, using \"" + .level + "\" instead.")
        {{"}}"}}

        .message, err = string(event.message)
        if err != null || is_empty(.message) {{"{{"}}
          .errors = push(.errors, "Message not found.")
        {{"}}"}}
      {{"}}"}}

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
			. |= parse_regex!(.file, r'^/kubedoop/log/(?P<container>.*?)/(?P<file>.*?)$')
			del(.source_type)
	extended_logs:
		inputs:
			- extended_logs_*
		type: remap
		source: |
			.namespace = "{{.Namespace}}"
			.cluster = "{{.Cluster}}"
			.role = "{{.RoleName}}"
			.roleGroup = "{{.RoleGroupName}}"
sinks:
	aggregator:
		inputs:
			- extended_logs
		type: vector
		address: "{{.VectorAggregatorAddress}}"
`
	parser := config.TemplateParser{
		Value:    data,
		Template: tmpl,
	}

	str, err := parser.Parse()
	if err != nil {
		return "", err
	}
	str = util.IndentTab2Spaces(str)
	return str, nil
}

func vectorAggregatorDiscoveryURI(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string,
	discoveryConfigName string,
) *string {
	if discoveryConfigName != "" {
		cli := pkgclient.Client{Client: client}
		cm := &corev1.ConfigMap{}
		err := cli.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: discoveryConfigName}, cm)
		if err != nil {
			return nil
		}
		address := cm.Data["ADDRESS"]
		return &address
	}
	return nil
}
