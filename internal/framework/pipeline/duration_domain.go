package pipeline

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

const durationInputLimit int64 = 128

func validateDurationJSON(data json.RawMessage, path string) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil || utf8.RuneCountInString(value) > int(durationInputLimit) {
		return fmt.Errorf("%s: duration must be a string of at most %d characters", path, durationInputLimit)
	}
	_, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s: invalid duration: %w", path, err)
	}
	return nil
}

func validateShutdownDuration(duration time.Duration) error {
	if duration < 0 || duration%time.Second != 0 {
		return fmt.Errorf("graceful shutdown duration must be nonnegative and an exact number of seconds")
	}
	return nil
}
