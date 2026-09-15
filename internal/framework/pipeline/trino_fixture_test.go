package pipeline

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
	NodeEnvironment string `json:"nodeEnvironment"`
}

type TrinoConfig struct {
	HTTPPort int32 `json:"httpPort"`
	// Empty means no external reference; nonempty resolves catalogs.json in the CR namespace.
	CatalogConfigMapName string `json:"catalogConfigMapName"`
}

// TrinoFacts is already-resolved group data. The generator performs no API reads.
type TrinoFacts struct {
	Catalogs map[string]map[string]string `json:"catalogs"`
}

// TrinoDefinition is a test-only adapter for the existing Trino validation slice.
// Runtime operator adoption remains U04; this fixture imports no prototype package.
func TrinoDefinition() framework.ProductDefinition[TrinoConfig, TrinoClusterConfig, TrinoFacts] {
	defaults := framework.Config[TrinoConfig]{
		Common: CommonConfig{
			GracefulShutdownTimeout: metav1.Duration{Duration: 30 * time.Second},
			Resources: Resources{
				CPU:    CPU{Min: resource.MustParse("500m"), Max: resource.MustParse("2")},
				Memory: Memory{Limit: resource.MustParse("1536Mi")},
			},
			Logging: Logging{EnableVectorAgent: true, Containers: map[string]ContainerLogging{
				trinoName: {Console: Logger{Level: trinoOffLevel}, File: Logger{Level: trinoTraceLevel},
					Loggers: map[string]Logger{trinoRootLogger: {Level: trinoInfoLevel}, trinoLoggerName: {Level: trinoInfoLevel}}},
			}},
		},
		Product: TrinoConfig{HTTPPort: 8080},
	}
	return framework.ProductDefinition[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
		ClusterConfigDefaults: TrinoClusterConfig{NodeEnvironment: "design_validation"},
		ImageDefaults: ImageConfig{Repo: "quay.io/zncdatadev", ProductVersion: "476",
			KubedoopVersion: "0.0.0-dev", PullPolicy: corev1.PullIfNotPresent},
		Name: trinoName, Roles: map[string]framework.RoleDefinition[TrinoConfig]{
			trinoCoordinatorRole: {Config: defaults,
				RoleConfig: RoleConfig{PodDisruptionBudget: PodDisruptionBudgetConfig{Enabled: true, MaxUnavailable: 0}}},
			trinoWorkerRole: {Config: defaults,
				RoleConfig: RoleConfig{PodDisruptionBudget: PodDisruptionBudgetConfig{Enabled: true, MaxUnavailable: 1}}},
		},
		ValidateInput: validateTrinoInput, GenerateGroup: generateTrino, GenerateCluster: trinoDiscovery,
		ValidateFinal: validateTrinoFinal,
	}
}

func validateTrinoInput(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) error {
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

func trinoFile(name string, values map[string]PropertyValue) File {
	return File{Directory: trinoConfigDirectory, Path: name,
		Content: KeyValues{Codec: PropertiesCodec{}, Values: values}}
}

func generateTrino(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) (
	RuntimeDescription, error,
) {
	uri, err := trinoCoordinatorURI(in)
	if err != nil {
		return RuntimeDescription{}, err
	}
	// This is the previous deployment fixture's 75% policy, not sizing guidance.
	bytes, exact := in.Config.Common.Resources.Memory.Limit.AsInt64()
	heapMi := (bytes / (1024 * 1024)) * 3 / 4
	if !exact || heapMi <= 0 {
		return RuntimeDescription{}, fmt.Errorf("memory must yield a positive integral fixture heap")
	}
	uid, nonRoot := int64(1001), true // This fixture's execution identity, not a universal image default.
	configAccess := DirectoryAccess{Directory: trinoConfigDirectory, MountPath: "/etc/trino", ReadOnly: true}
	dataAccess := DirectoryAccess{Directory: trinoDataDirectory, MountPath: "/var/trino/data"}
	logAccess := DirectoryAccess{Directory: trinoLogDirectory, MountPath: "/kubedoop/log/trino"}
	logFile := trinoServerLog
	logging, err := resolveTrinoLogging(in.Config.Common.Logging)
	if err != nil {
		return RuntimeDescription{}, err
	}
	configuration := map[string]PropertyValue{
		"coordinator":           Literal(strconv.FormatBool(in.Group.Role == trinoCoordinatorRole)),
		"http-server.http.port": Literal(strconv.Itoa(int(in.Config.Product.HTTPPort))),
		"discovery.uri":         Literal(uri),
		"log.enable-console":    Literal(strconv.FormatBool(logging.Console)),
	}
	if logging.File {
		configuration["log.path"] = Literal(path.Join(logAccess.MountPath, logFile))
		configuration["log.format"] = Literal("JSON")
	}
	r := RuntimeDescription{
		ConfigDirectory: trinoConfigDirectory,
		Main: Process{
			Name:    trinoName,
			Command: []string{"/kubedoop/trino-server/bin/launcher"},
			Args:    []string{"--etc-dir=" + configAccess.MountPath, trinoRunArgument},
			Identity: &corev1.SecurityContext{
				RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &nonRoot,
			},
			Access: []DirectoryAccess{configAccess, dataAccess, logAccess},
		},
		Directories: []Directory{{Name: trinoConfigDirectory}, {Name: trinoDataDirectory}, {Name: trinoLogDirectory}},
		SharedGroup: &uid,
		Endpoints:   []Endpoint{{Name: trinoHTTPEndpoint, Port: in.Config.Product.HTTPPort}},
		Files: []File{
			trinoFile("config.properties", configuration),
			{Directory: trinoConfigDirectory, Path: trinoJVMFile, Content: Lines{fmt.Sprintf("-Xmx%dm", heapMi)}},
			trinoFile(trinoNodeFile, map[string]PropertyValue{
				trinoNodeIDKey: PodNameBinding{}, trinoEnvironmentKey: Literal(in.ClusterConfig.NodeEnvironment),
				"node.data-dir": Literal(dataAccess.MountPath),
			}),
		},
	}
	if logging.File {
		r.LogOutputs = []LogOutput{{Container: trinoName, Directory: logAccess.Directory, RelativePath: logFile}}
	}
	r.Files = append(r.Files, trinoFile("log.properties", logging.Levels))
	for _, name := range sortedKeys(in.Facts.Catalogs) {
		if !relativeFile(name) || strings.Contains(name, "/") {
			return RuntimeDescription{}, fmt.Errorf("catalog name must be a single file component: %q", name)
		}
		values := map[string]PropertyValue{}
		for key, value := range in.Facts.Catalogs[name] {
			values[key] = Literal(value)
		}
		r.Files = append(r.Files, trinoFile("catalog/"+name+".properties", values))
	}
	return r, nil
}

// Shared output is explicit about waiting versus withdrawing all resources.
func trinoDiscovery(in framework.ClusterOutputInput[TrinoClusterConfig, TrinoFacts]) (ClusterOutput, error) {
	for _, group := range in.Groups {
		if group.Group.Role != trinoCoordinatorRole || group.Error != "" {
			continue
		}
		if group.Facts != nil && group.Facts.State != FactsResolved {
			return ClusterOutput{State: ClusterOutputPending, Reason: "coordinator facts unavailable"}, nil
		}
		for _, endpoint := range group.GeneratedEndpoints {
			if endpoint.Name == trinoHTTPEndpoint {
				return ClusterOutput{State: ClusterOutputReady, ConfigMaps: []corev1.ConfigMap{{
					ObjectMeta: metav1.ObjectMeta{Name: in.Cluster.Name + "-discovery", Namespace: in.Cluster.Namespace},
					Data:       map[string]string{"TRINO_URI": fmt.Sprintf("http://%s:%d", group.Group.ServiceDNS(), endpoint.Port)},
				}}}, nil
			}
		}
	}
	return ClusterOutput{}, fmt.Errorf("coordinator generated HTTP endpoint unavailable")
}
