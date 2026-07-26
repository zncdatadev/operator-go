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

package config

import (
	"errors"
	"fmt"
)

// ErrNoFormat reports a generator that was constructed without a format. It is a programming
// error rather than bad user input, so it carries no format or file context.
var ErrNoFormat = errors.New("no configuration format configured")

// UnsupportedParseError reports a parse attempt against a format that can only emit. Marshal is
// the whole required contract (ConfigMarshaler); the parsing half is optional, so a registered
// format may legitimately lack it. The message names the format — and the file when the caller
// knows one — because that is what tells an operator author which adapter has to grow an
// Unmarshal method.
type UnsupportedParseError struct {
	// Format identifies the offending format: the registered file extension, the requested
	// ConfigFormatType, or the adapter's Go type when the caller knows no better name.
	Format string

	// File is the configuration file being parsed. It is empty when the content has no filename,
	// as with ConfigGenerator.Parse.
	File string
}

func (e *UnsupportedParseError) Error() string {
	target := "configuration content"
	if e.File != "" {
		target = fmt.Sprintf("config file %q", e.File)
	}
	return fmt.Sprintf("cannot parse %s: the %s format only implements ConfigMarshaler, not ConfigUnmarshaler",
		target, e.Format)
}

// serializeError wraps an adapter failure on the emit path with the format it came from.
func serializeError(format string, err error) error {
	return fmt.Errorf("failed to serialize %s configuration: %w", format, err)
}

// parseError wraps an adapter failure on the read path with the format it came from.
func parseError(format string, err error) error {
	return fmt.Errorf("failed to parse %s configuration: %w", format, err)
}
