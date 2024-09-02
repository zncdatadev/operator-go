package productlogging

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// calculate_log_volume_size_limit calculates the log volume size limit based on the given max log file sizes.
// The limit is calculated by summing up all the given sizes, scaling the result to MEBI and multiplying it by 3.0.
// The result is then ceiled to avoid bulky numbers due to floating-point arithmetic.
func CalculateLogVolumeSizeLimit(maxLogFilesSize []resource.Quantity) resource.Quantity {
	logVolumeSizeLimit := resource.Quantity{}
	for _, q := range maxLogFilesSize {
		logVolumeSizeLimit.Add(q)
	}
	// According to the reasons mentioned in the function documentation, the multiplier must be
	// greater than 2. Manual tests with ZooKeeper 3.8 in an OpenShift cluster showed that 3 is
	// absolutely sufficient.
	logVolumeSizeLimit.Set(logVolumeSizeLimit.Value() * 3.0)
	return logVolumeSizeLimit
}

func GetLoggerLevel(condition bool, trueValFunc func() string, defaultVal string) string {
	if condition {
		trueVal := trueValFunc()
		if strings.TrimSpace(trueVal) != "" {
			return trueVal
		}
	}
	return defaultVal
}
