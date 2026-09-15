package framework

import (
	corev1 "k8s.io/api/core/v1"
)

type RuntimeDescription struct {
	ConfigDirectory string
	Main            Process
	Files           []File
	Directories     []Directory
	SharedGroup     *int64
	Endpoints       []Endpoint
	LogOutputs      []LogOutput
	// Initializers run in declaration order after file materialization and before Main.
	Initializers []Process
	Coordination *WorkloadCoordination
}

type Process struct {
	Name, Image                                 string
	Command, Args                               []string
	Env                                         []corev1.EnvVar
	Identity                                    *corev1.SecurityContext
	Access                                      []DirectoryAccess
	Lifecycle                                   *corev1.Lifecycle
	StartupProbe, ReadinessProbe, LivenessProbe *corev1.Probe
}

// Directory names a runtime directory. Data binds the one supported product
// data directory to effective config.resources.storage. All other directories
// are ephemeral. Configuration files and logs require separate directories.
type Directory struct {
	Secret   *SecretVolume
	Listener *ListenerVolume
	Name     string
	Data     bool
}

type DirectoryAccess struct {
	Directory, MountPath string
	ReadOnly             bool
}

type File struct {
	Directory, Path string
	Content         FileContent
}

// FileContent is the closed set of supported declaration forms. Use variant
// values, not pointers; the framework preserves structure until overrides finish.
type FileContent interface{ fileContent() }

type KeyValues struct {
	Codec  PropertyCodec
	Values map[string]PropertyValue
}

type Lines []string
type Text string

func (KeyValues) fileContent() {}
func (Lines) fileContent()     {}
func (Text) fileContent()      {}

// PropertyCodec encodes resolved property values as product data. A custom Go
// codec does not imply that the runtime materialization helper can execute it.
type PropertyCodec interface {
	Encode(map[string]string) (string, error)
}

// PropertyValue keeps a literal distinct from a deferred per-Pod value.
type PropertyValue interface{ propertyValue() }

type Literal string
type PodNameBinding struct{}

func (Literal) propertyValue()        {}
func (PodNameBinding) propertyValue() {}

type Endpoint struct {
	Name string
	Port int32
}

// LogOutput declares a file the product actually produces. Collection is chosen
// once by the framework from effective Logging.EnableVectorAgent, not per output.
type LogOutput struct {
	Container, Directory, RelativePath string
}

type CheckState string

const (
	Consistent CheckState = "consistent"
	Conflict   CheckState = "conflict"
	Unknown    CheckState = "unknown"
)

type Check struct {
	Subject string     `json:"subject"`
	State   CheckState `json:"state"`
	Reason  string     `json:"reason,omitempty"`
}
