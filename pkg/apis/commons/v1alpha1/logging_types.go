package v1alpha1

type LoggingSpec struct {
	// +kubebuilder:validation:Optional
	Containers map[string]LoggingConfigSpec `json:"containers,omitempty"`
	// +kubebuilder:validation:Optional
	EnableVectorAgent *bool `json:"enableVectorAgent,omitempty"`
}

type LoggingConfigSpec struct {
	// +kubebuilder:validation:Optional
	Loggers map[string]*LogLevelSpec `json:"loggers,omitempty"`

	// +kubebuilder:validation:Optional
	Console *LogLevelSpec `json:"console,omitempty"`

	// +kubebuilder:validation:Optional
	File *LogLevelSpec `json:"file,omitempty"`
}

// LogLevelSpec
// level mapping if app log level is not standard
//   - FATAL -> CRITICAL
//   - ERROR -> ERROR
//   - WARN -> WARNING
//   - INFO -> INFO
//   - DEBUG -> DEBUG
//   - TRACE -> DEBUG
//
// Default log level is INFO
type LogLevelSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="INFO"
	// +kubebuilder:validation:Enum=FATAL;ERROR;WARN;INFO;DEBUG;TRACE
	Level string `json:"level,omitempty"`
}
