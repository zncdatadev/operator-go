package product

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const trinoPython = "python3"

// The initialization check is idempotent. It checks the materialized inputs and
// writable directories without creating an initialization-completed marker that
// could outlive a Pod or be mistaken for a product migration receipt.
const trinoInitialize = `import os,pathlib,tempfile
for name in ('config.properties','node.properties','jvm.config'):
 p=pathlib.Path('/etc/trino')/name
 if not p.is_file(): raise RuntimeError('missing materialized input: '+name)
 with p.open('rb') as source: source.read(1)
for directory in ('/var/trino/data','/kubedoop/log/trino'):
 with tempfile.TemporaryFile(dir=directory) as probe:
  probe.write(b'kubedoop-initialization-check'); probe.flush(); os.fsync(probe.fileno())
print('KUBEDOOP_INITIALIZED',flush=True)
`

const trinoReady = `import json,sys,urllib.request
with urllib.request.urlopen('http://127.0.0.1:'+sys.argv[1]+'/v1/info',timeout=2) as response:
 info=json.load(response)
 if info.get('starting') is not False: raise RuntimeError('Trino is starting')
`

// This protocol waits for the exact JVM birth identity after accepting shutdown.
// A successful PUT is not graceful completion. If the JVM exits, kubelet may
// terminate this hook's exec process; hook and main-process exit evidence differ.
// The explicit deadline fails the hook and leaves the final termination decision
// to kubelet's declared Pod budget. It never sends kill or treats network errors
// as a successful business shutdown.
const trinoShutdown = `import base64,glob,pathlib,sys,time,urllib.request
port,user,budget=sys.argv[1],sys.argv[2],int(sys.argv[3])
deadline=time.monotonic()+budget
candidates=[]
for name in glob.glob('/proc/[0-9]*/cmdline'):
 try:
  if b'io.trino.server.TrinoServer' in pathlib.Path(name).read_bytes().split(b'\0'):
   candidates.append(pathlib.Path(name).parent)
 except (FileNotFoundError,PermissionError,ProcessLookupError): pass
if len(candidates)!=1: raise RuntimeError('cannot establish exact JVM identity')
process=candidates[0]
def birth():
 try:
  fields=(process/'stat').read_text().rsplit(')',1)[1].split()
  return fields[19] if fields[0]!='Z' else None
 except (FileNotFoundError,ProcessLookupError): return None
started=birth()
if started is None: raise RuntimeError('JVM exited before shutdown request')
headers={'Content-Type':'application/json','X-Trino-User':user}
if len(sys.argv)>4:
 root=pathlib.Path(sys.argv[4])
 username=(root/'username').read_text().strip(); password=(root/'password').read_text().rstrip('\r\n')
 if not username or not password: raise RuntimeError('shutdown credentials are incomplete')
 headers['Authorization']='Basic '+base64.b64encode((username+':'+password).encode()).decode()
 headers['X-Trino-User']=username
request=urllib.request.Request('http://127.0.0.1:'+port+'/v1/info/state',data=b'"SHUTTING_DOWN"',method='PUT',headers=headers)
with urllib.request.urlopen(request,timeout=min(5,budget)) as response:
 if response.status!=200: raise RuntimeError('shutdown was not accepted')
 with open('/proc/1/fd/1','w') as output:
  print('KUBEDOOP_SHUTDOWN_ACCEPTED',response.status,'jvm_pid='+process.name,'starttime='+started,flush=True,file=output)
while birth()==started:
 if time.monotonic()>=deadline: raise TimeoutError('JVM did not exit within shutdown budget')
 time.sleep(.25)
`

func configureTrinoLifecycle(r *framework.RuntimeDescription, in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) {
	port := strconv.Itoa(int(in.Config.Product.HTTPPort))
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{
		Command: []string{trinoPython, "-c", trinoReady, port}}}, TimeoutSeconds: 3, PeriodSeconds: 2, FailureThreshold: 120}
	r.Main.StartupProbe = probe.DeepCopy()
	r.Main.ReadinessProbe = probe.DeepCopy()
	r.Main.ReadinessProbe.FailureThreshold = 3
	r.Main.LivenessProbe = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/v1/info", Port: intstr.FromString("http")}}, PeriodSeconds: 10, FailureThreshold: 6}
	r.Initializers = []framework.Process{{Name: "initialize-trino", Image: r.Main.Image,
		Command: []string{trinoPython, "-c", trinoInitialize}, Identity: r.Main.Identity.DeepCopy(),
		Access: append([]framework.DirectoryAccess(nil), r.Main.Access...)}}
	r.Coordination = &framework.WorkloadCoordination{ProgressDeadline: metav1.Duration{Duration: 10 * time.Minute}}
	if in.Group.Role == trinoCoordinatorRole {
		r.Coordination.ShutdownPriority = 100
	}
	if in.Group.Role == trinoWorkerRole && (in.Config.Product.ShutdownUser != "" || in.Config.Product.ShutdownCredentialsSecret != "") {
		budget := int64(in.Config.Common.GracefulShutdownTimeout.Duration/time.Second) - 2
		credentials := in.Config.Product.ShutdownCredentialsSecret
		if credentials != "" {
			r.Directories = append(r.Directories, framework.Directory{Name: "shutdown-credentials", Secret: &framework.SecretVolume{SecretName: credentials}})
			r.Main.Access = append(r.Main.Access, framework.DirectoryAccess{Directory: "shutdown-credentials", MountPath: "/kubedoop/shutdown-credentials", ReadOnly: true})
		}
		r.Main.Lifecycle = &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{
			Command: []string{trinoPython, "-c", trinoShutdown, port, in.Config.Product.ShutdownUser, strconv.FormatInt(budget, 10)}}}}
		if credentials != "" {
			r.Main.Lifecycle.PreStop.Exec.Command = append(r.Main.Lifecycle.PreStop.Exec.Command, "/kubedoop/shutdown-credentials")
		}
	}
}

func validateTrinoLifecycle(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) error {
	user := in.Config.Product.ShutdownUser
	if strings.ContainsAny(user, "\r\n\x00") || len(user) > 256 || strings.TrimSpace(user) != user {
		return fmt.Errorf("shutdownUser must be a bounded HTTP identity without control characters")
	}
	if (user != "" || in.Config.Product.ShutdownCredentialsSecret != "") && in.Config.Common.GracefulShutdownTimeout.Duration < 10*time.Second {
		return fmt.Errorf("shutdownUser requires at least 10s gracefulShutdownTimeout")
	}
	return nil
}
