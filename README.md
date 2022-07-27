# PodChaosMonkey

**This tool is small PoC destinated to run minikube and not on a production
cluster.**

PodChaosMonkey is tool that deletes one pod at random in a particular
namespace on a schedule.

**Limitations/Assumptions:**

* Only running pods are taken into consideration.
* Running pods are not distinguished between `Terminating` and `NotTerminating`.
* There is no retry mechanism for the pod deletion. In case a pod is not deleted
for various reasons (not present in cache, already deleted, intermittent error),
the next deletion will happen at the next interval.

## Build

Golang `1.18` is required for building.

```bash
make build
```

## Build image

Use your favourite tool to build the image, e.g: podman:

```bash
podman build -t my.awesome.registry/podchaosmonkey:latest .
```

## Configuration flags

**-dry-run**

Use this flag if you want to see which pod would be deleted without
actually deleting the pods

**-label-selector**

Select sepecific pods in a namespace based on the pod labels.

**-deletion-interval**

Change the default deletion interval to a custom value, setting `15s` will
perform the deletion of a pod every 15s.

**-namespace**

Change the target namespace for pod deletion to a custom value, setting `abcd`
will perform the deletion in `abcd` namespace. /!\ remember to modify RBAC to
allow the `ServiceAccount` of `podchaosmonkey` to act in this namespace.

**Example:**

```bash
podchaosmonkey \
  --dry-run \
  -v=3 \
  --deletion-interval 15s \
  -label-selector "app.kubernetes.io/name=superapp"
```

## Deployment

### Kustomize

This has been tested with kustomize `v4.5.5`.

The deployment files assume the target namespace for pod deletion is `workloads`.

:warning: For the purpose of the demo, the deletion interval is set to `15s`,
this can be configured in `kustomize/podchaosmonkey/deployment.yaml`.

Set the image to use:

```bash
pushd kustomize/podchaosmonkey
kustomize edit set image podchaosmonkey=my.awesome.registry/podchaosmonkey:latest
popd
```

Deploy:

* podchaosmonkey in its respective namespace `podchaosmonkey`.
* the superapp workload in its respective namespace `workloads`.
* the required rbac in the `workloads` namespace to allow podchaosmonkey to do
its dirty work.

```bash
kustomize build kustomize/ | kubectl apply -f -
```

## Deploy in minikube

The following procedure was tested with minikube `1.26.0` and
Kubernetes `1.24.1`.

1. Start minikube

In order to consume the internal registry which exposes HTTP by default, start
minikube with the following flag to allow the consumption of the HTTP registry.
All private network ranges are added for convenience to accomodate every possible
configuration.

```bash
minikube start --addons=registry \
  --insecure-registry "10.0.0.0/8" \
  --insecure-registry "172.16.0.0/12" \
  --insecure-registry "192.168.0.0/16"
```

2. Deploy

The following command will take care of building the image and applying the
manifests to the cluster.

```bash
make minikube-deploy
```

## Usage

```
Usage of ./podchaosmonkey:
  -add_dir_header
    	If true, adds the file directory to the header of the log messages
  -alsologtostderr
    	log to standard error as well as files
  -deletion-interval duration
    	Sets the interval to trigger the deletion of a pod in the provided namespace (default 1h0m0s)
  -dry-run
    	Do not actually delete pod, logs only the pod that would be deleted otherwise
  -kubeconfig string
    	The path to the kubeconfig, default to in-cluster config if not provided (default "/home/noname/.kube/config")
  -label-selector string
    	Select specific pods to delete with this labels, format: key=val,key2=val2
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -log_file string
    	If non-empty, use this log file
  -log_file_max_size uint
    	Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
  -logtostderr
    	log to standard error instead of files (default true)
  -namespace string
    	Namespace to watch (default "workloads")
  -one_output
    	If true, only write logs to their native severity level (vs also writing to each lower severity level)
  -skip_headers
    	If true, avoid header prefixes in the log messages
  -skip_log_headers
    	If true, avoid headers when opening log files
  -stderrthreshold value
    	logs at or above this threshold go to stderr (default 2)
  -v value
    	number for the log level verbosity
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
```
