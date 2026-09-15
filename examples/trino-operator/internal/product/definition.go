package product

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	trinoName            = "trino"
	trinoLauncher        = "/kubedoop/trino-server/bin/launcher"
	trinoCoordinatorRole = "coordinators"
	trinoWorkerRole      = "workers"
	trinoConfigDirectory = "config"
	trinoLogDirectory    = "log"
	trinoServerLog       = "server.json"
	trinoRunArgument     = "run"
	trinoHTTPEndpoint    = "http"
	trinoInfoLevel       = "INFO"
	trinoDataDirectory   = "data"
	trinoJVMFile         = "jvm.config"
	trinoEnvironmentKey  = "node.environment"
	trinoHTTPCheck       = "trino.http"
	trinoExecutionCheck  = "trino.execution"
	trinoNodeFile        = "node.properties"
	trinoNodeIDKey       = "node.id"
)

var trinoEnvironmentPattern = regexp.MustCompile(`^[a-z0-9][_a-z0-9]*$`)

// TrinoClusterConfig is cluster-wide product intent, outside role inheritance and facts.
type TrinoClusterConfig struct {
	ListenerClass   string `json:"listenerClass"`
	NodeEnvironment string `json:"nodeEnvironment"`
	TLSSecret       string `json:"tlsSecret"`
	TLSSecretClass  string `json:"tlsSecretClass"`
	InternalSecret  string `json:"internalSecret"`
}

type TrinoConfig struct {
	Hive     HiveCatalog `json:"hive"`
	HTTPPort int32       `json:"httpPort"`
	// Explicit local shutdown identity; its management authorization must already exist.
	ShutdownUser              string `json:"shutdownUser"`
	ShutdownCredentialsSecret string `json:"shutdownCredentialsSecret"`
	// Empty means no external reference; nonempty resolves catalogs.json in the CR namespace.
	CatalogConfigMapName string `json:"catalogConfigMapName"`
}

// TrinoFacts is already-resolved group data. The generator performs no API reads.
type TrinoFacts struct {
	S3             framework.ResolvedS3Connection `json:"s3"`
	Authentication AuthenticationFacts            `json:"authentication"`
	Catalogs       map[string]map[string]string   `json:"catalogs"`
}

// Definition declares Trino 476 input, native files and runtime intent.
// The framework owns configuration folding, helpers, resources and reconciliation.
func Definition() framework.ProductDefinition[TrinoConfig, TrinoClusterConfig, TrinoFacts] {
	defaults := framework.Config[TrinoConfig]{
		Common: framework.CommonConfig{
			GracefulShutdownTimeout: metav1.Duration{Duration: 30 * time.Second},
			Resources: framework.Resources{
				CPU:    framework.CPU{Min: resource.MustParse("500m"), Max: resource.MustParse("2")},
				Memory: framework.Memory{Limit: resource.MustParse("1536Mi")},
			},
			Logging: framework.Logging{EnableVectorAgent: true, Containers: map[string]framework.ContainerLogging{
				trinoName: {Console: framework.Logger{Level: trinoOffLevel}, File: framework.Logger{Level: trinoTraceLevel},
					Loggers: map[string]framework.Logger{trinoRootLogger: {Level: trinoInfoLevel}, trinoLoggerName: {Level: trinoInfoLevel}}},
			}},
		},
		Product: TrinoConfig{HTTPPort: 8080},
	}
	return framework.ProductDefinition[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
		ClusterConfigDefaults: TrinoClusterConfig{NodeEnvironment: "kubedoop"},
		ImageDefaults: framework.ImageConfig{Repo: "quay.io/zncdatadev", ProductVersion: "476",
			KubedoopVersion: "0.0.0-dev", PullPolicy: corev1.PullIfNotPresent},
		Name: trinoName, Roles: map[string]framework.RoleDefinition[TrinoConfig]{
			trinoCoordinatorRole: {Config: defaults,
				RoleConfig: framework.RoleConfig{PodDisruptionBudget: framework.PodDisruptionBudgetConfig{Enabled: true, MaxUnavailable: 0}}},
			trinoWorkerRole: {Config: defaults,
				RoleConfig: framework.RoleConfig{PodDisruptionBudget: framework.PodDisruptionBudgetConfig{Enabled: true, MaxUnavailable: 1}}},
		},
		ValidateInput: validateTrinoInput, GenerateGroup: generateTrino, GenerateCluster: trinoDiscovery,
		ValidateFinal: validateTrinoFinal,
	}
}

func validateTrinoInput(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) error {
	if err := validateTrinoLifecycle(in); err != nil {
		return err
	}
	if !trinoEnvironmentPattern.MatchString(in.ClusterConfig.NodeEnvironment) {
		return fmt.Errorf("clusterConfig.nodeEnvironment must match [a-z0-9][_a-z0-9]*")
	}
	if in.Config.Product.HTTPPort < 1 || in.Config.Product.HTTPPort > 65535 {
		return fmt.Errorf("httpPort must be between 1 and 65535")
	}
	coordinators := 0
	for _, group := range in.Topology {
		if group.Group.Role == trinoCoordinatorRole {
			coordinators++
			if group.Group.Replicas != 1 {
				return fmt.Errorf("this Trino example requires one coordinator replica")
			}
		}
	}
	if coordinators != 1 {
		return fmt.Errorf("this Trino example requires one coordinator group")
	}
	if _, err := resolveTrinoLogging(in.Config.Common.Logging); err != nil {
		return err
	}
	return ValidateTrinoCatalogs(in.Facts.Catalogs)
}

// ValidateTrinoCatalogs is shared by external resolution and the pure generator.
// It validates the supported catalog shape, not plugin availability or credentials.
func ValidateTrinoCatalogs(catalogs map[string]map[string]string) error {
	for _, name := range sortedKeys(catalogs) {
		if !relativeFile(name) || strings.Contains(name, "/") || !utf8.ValidString(name) {
			return fmt.Errorf("catalog name must be a single valid file component")
		}
		values := catalogs[name]
		if values == nil || strings.TrimSpace(values["connector.name"]) == "" {
			return fmt.Errorf("catalog requires a nonempty connector.name")
		}
		for key, value := range values {
			if key == "" || !utf8.ValidString(key) || !utf8.ValidString(value) {
				return fmt.Errorf("catalog property keys must be nonempty and properties must contain valid UTF-8")
			}
		}
	}
	return nil
}

func trinoCoordinatorURI(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) (string, error) {
	for _, group := range in.Topology {
		if group.Group.Role != trinoCoordinatorRole {
			continue
		}
		if group.Error != "" || group.Config == nil {
			return "", fmt.Errorf("coordinator input unavailable: %s", group.Error)
		}
		return fmt.Sprintf("http://%s:%d", group.Group.ServiceDNS(), group.Config.Product.HTTPPort), nil
	}
	return "", fmt.Errorf("coordinator input unavailable: no coordinator group")
}

func trinoFile(name string, values map[string]framework.PropertyValue) framework.File {
	return framework.File{Directory: trinoConfigDirectory, Path: name,
		Content: framework.KeyValues{Codec: framework.PropertiesCodec{}, Values: values}}
}

func generateTrino(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) (
	framework.RuntimeDescription, error,
) {
	uri, err := trinoCoordinatorURI(in)
	if err != nil {
		return framework.RuntimeDescription{}, err
	}
	// Reserve one quarter of the declared memory for non-heap JVM usage.
	bytes, exact := in.Config.Common.Resources.Memory.Limit.AsInt64()
	heapMi := (bytes / (1024 * 1024)) * 3 / 4
	if !exact || heapMi <= 0 {
		return framework.RuntimeDescription{}, fmt.Errorf("memory must yield a positive integral heap")
	}
	uid, nonRoot := int64(1001), true // The selected Kubedoop Trino image runs with this identity.
	configAccess := framework.DirectoryAccess{Directory: trinoConfigDirectory, MountPath: "/etc/trino", ReadOnly: true}
	dataAccess := framework.DirectoryAccess{Directory: trinoDataDirectory, MountPath: "/var/trino/data"}
	logAccess := framework.DirectoryAccess{Directory: trinoLogDirectory, MountPath: "/kubedoop/log/trino"}
	logFile := trinoServerLog
	logging, err := resolveTrinoLogging(in.Config.Common.Logging)
	if err != nil {
		return framework.RuntimeDescription{}, err
	}
	configuration := map[string]framework.PropertyValue{
		"coordinator":           framework.Literal(strconv.FormatBool(in.Group.Role == trinoCoordinatorRole)),
		"http-server.http.port": framework.Literal(strconv.Itoa(int(in.Config.Product.HTTPPort))),
		"discovery.uri":         framework.Literal(uri),
		"log.enable-console":    framework.Literal(strconv.FormatBool(logging.Console)),
	}
	if in.Group.Role == trinoCoordinatorRole {
		configuration["node-scheduler.include-coordinator"] = framework.Literal("false")
	}
	if logging.File {
		configuration["log.path"] = framework.Literal(path.Join(logAccess.MountPath, logFile))
		configuration["log.format"] = framework.Literal("JSON")
	}
	r := framework.RuntimeDescription{
		ConfigDirectory: trinoConfigDirectory,
		Main: framework.Process{
			Name:    trinoName,
			Command: []string{trinoLauncher},
			Args:    []string{"--etc-dir=" + configAccess.MountPath, trinoRunArgument},
			Env: []corev1.EnvVar{{Name: "TRINO_NODE_ID", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"},
			}}},
			Identity: &corev1.SecurityContext{
				RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &nonRoot,
			},
			Access: []framework.DirectoryAccess{configAccess, dataAccess, logAccess},
		},
		Directories: []framework.Directory{{Name: trinoConfigDirectory}, {Name: trinoDataDirectory, Data: true}, {Name: trinoLogDirectory}},
		SharedGroup: &uid,
		Endpoints:   []framework.Endpoint{{Name: trinoHTTPEndpoint, Port: in.Config.Product.HTTPPort}},
		Files: []framework.File{
			trinoFile("config.properties", configuration),
			{Directory: trinoConfigDirectory, Path: trinoJVMFile, Content: framework.Lines{fmt.Sprintf("-Xmx%dm", heapMi)}},
			trinoFile(trinoNodeFile, map[string]framework.PropertyValue{
				trinoNodeIDKey: framework.Literal("${ENV:TRINO_NODE_ID}"), trinoEnvironmentKey: framework.Literal(in.ClusterConfig.NodeEnvironment),
				"node.data-dir": framework.Literal(dataAccess.MountPath),
			}),
		},
	}
	configureTrinoLifecycle(&r, in)
	configureTrinoListener(&r, in)
	if logging.File {
		r.LogOutputs = []framework.LogOutput{{Container: trinoName, Directory: logAccess.Directory, RelativePath: logFile}}
	}
	r.Files = append(r.Files, trinoFile("log.properties", logging.Levels))
	for _, name := range sortedKeys(in.Facts.Catalogs) {
		if !relativeFile(name) || strings.Contains(name, "/") {
			return framework.RuntimeDescription{}, fmt.Errorf("catalog name must be a single file component: %q", name)
		}
		values := map[string]framework.PropertyValue{}
		for key, value := range in.Facts.Catalogs[name] {
			values[key] = framework.Literal(value)
		}
		r.Files = append(r.Files, trinoFile("catalog/"+name+".properties", values))
	}
	if err := configureTrinoAuthentication(&r, in); err != nil {
		return framework.RuntimeDescription{}, err
	}
	if err := configureTrinoS3(&r, in); err != nil {
		return framework.RuntimeDescription{}, err
	}
	return r, nil
}

// Shared output is explicit about waiting versus withdrawing all resources.
func trinoDiscovery(in framework.ClusterOutputInput[TrinoClusterConfig, TrinoFacts]) (framework.ClusterOutput, error) {
	for _, group := range in.Groups {
		if group.Group.Role != trinoCoordinatorRole {
			continue
		}
		if group.Error != "" {
			return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "coordinator input unavailable"}, nil
		}
		if group.Facts != nil && group.Facts.State != framework.FactsResolved {
			return framework.ClusterOutput{State: framework.ClusterOutputPending, Reason: "coordinator facts unavailable"}, nil
		}
		return trinoGroupDiscovery(in.Cluster, group)

	}
	return framework.ClusterOutput{State: framework.ClusterOutputReady}, nil
}
