package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	typev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	namespaceDefault        = "workloads"
	deletionIntervalDefault = 1 * time.Hour
	dryRunDefault           = false
)

const (
	informerResync = 24 * time.Hour
)

type podChaosMonkey struct {
	cache  cache.Store
	client typev1.PodInterface
	dryRun bool
}

// New returns a new podChaosMonkey
func New(client kubernetes.Interface, informerFactory kubeinformers.SharedInformerFactory, namespace string, dryRun bool) podChaosMonkey {
	return podChaosMonkey{
		cache:  informerFactory.Core().V1().Pods().Informer().GetStore(),
		client: client.CoreV1().Pods(namespace),
		dryRun: dryRun,
	}
}

// logLevelFromFlag converts the value of the -v flag
// to a compatible value for zapcore.Level
func logLevelFromFlag(logLevelFlag string) (int, error) {
	logLevel, err := strconv.Atoi(logLevelFlag)
	if err != nil {
		return 0, fmt.Errorf("invalid log level: %s", logLevelFlag)
	}
	if logLevel > 0 {
		logLevel = -1 * logLevel
	}

	return logLevel, nil
}

func newClientset(kubeconfig string) (*kubernetes.Clientset, error) {
	var err error
	var config *rest.Config

	// use local kubeconfig if provided by user
	if kubeconfig != "" {
		kubehome := filepath.Join(kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubehome)
		if err != nil {
			return nil, fmt.Errorf("error loading kubeconfig: %w", err)
		}
	} else {
		// use in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error loading in-cluster configuration: %w", err)
		}
	}

	return kubernetes.NewForConfig(config)
}

// newInformerFactory creates a shared informer to watch the pods in the provided namespace.
func newInformerFactory(client kubernetes.Interface, namespace string) kubeinformers.SharedInformerFactory {
	return kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		informerResync,
		kubeinformers.WithNamespace(namespace),
		kubeinformers.WithTweakListOptions(func(list *metav1.ListOptions) {
			list.FieldSelector = "status.phase=Running"
		}),
	)
}

func main() {
	var (
		namespace        string
		kubeconfig       string
		deletionInterval time.Duration
		dryRun           bool
	)

	klog.InitFlags(nil)

	// TODO: add specific labels
	flag.StringVar(&kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "The path to the kubeconfig, default to in-cluster config if not provided")
	flag.StringVar(&namespace, "namespace", namespaceDefault, "Namespace to watch")
	flag.DurationVar(&deletionInterval, "deletion-interval", deletionIntervalDefault, "Sets the interval to trigger the deletion of a pod in the provided namespace")
	flag.BoolVar(&dryRun, "dry-run", dryRunDefault, "Do not actually delete pod, logs only the pod that would be deleted otherwise")

	flag.Parse()

	// configure logging
	logLevelFlag := flag.CommandLine.Lookup("v")
	if logLevelFlag == nil {
		panic("log level can not be nil")
	}

	logLevel, err := logLevelFromFlag(logLevelFlag.Value.String())
	if err != nil {
		panic("the provided log level is invalid")
	}

	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(zapcore.Level(logLevel)))
	z, err := zc.Build()
	if err != nil {
		panic(fmt.Errorf("unrecoverable error during configuration of the logger: %w", err))
	}
	klog.SetLogger(zapr.NewLogger(z))

	client, err := newClientset(kubeconfig)
	if err != nil {
		klog.Fatal(err)
	}

	klog.InfoS("podchaosmonkey started")

	// start the informer
	stopCh := handleSignals()
	informerFactory := newInformerFactory(client, namespace)
	go informerFactory.Start(stopCh)

	podChaosMonkey := New(client, informerFactory, namespace, dryRun)

	for {
		select {
		case <-time.After(deletionInterval):
			err := podChaosMonkey.deleteRandomPod(getRandomPodKey)
			if err != nil {
				klog.ErrorS(err, "and occured during pod deletion")
			}
		case <-stopCh:
			klog.InfoS("termination signal received, closing podchaosmonkey gracefully")
			os.Exit(0)
		}
	}
}

// deleteRandomPod deletes pods randomly from an informer cache.
func (p podChaosMonkey) deleteRandomPod(selectorFn func([]string) string) error {
	ctx := context.Background()

	fmt.Println(p.cache.ListKeys())
	podKeys := p.cache.ListKeys()
	if len(podKeys) == 0 {
		klog.V(3).InfoS("no running pod found in namespace")
		return nil
	}

	podKey := selectorFn(podKeys)
	obj, found, err := p.cache.GetByKey(podKey)
	if err != nil {
		return fmt.Errorf("fetching the pod from cache failed: %w", err)
	}

	if !found {
		klog.InfoS("pod not found in cache, skipping deletion")
		return nil
	}

	klog.V(3).InfoS("pod candidate found for deletion", "pod", podKey)

	// should never occur but we check just in case
	pod, isPod := obj.(*corev1.Pod)
	if !isPod {
		return fmt.Errorf("unable to convert interface to pod: type assertion failed")
	}

	deleteOptions := metav1.DeleteOptions{}
	if p.dryRun {
		deleteOptions.DryRun = []string{"All"}
	}

	klog.InfoS("deleting pod", "pod", podKey)
	err = p.client.Delete(ctx, pod.GetName(), deleteOptions)
	if err != nil {
		if apierror.IsNotFound(err) {
			klog.InfoS("pod candidate was already deleted", "pod", podKey)
			return nil
		}

		return fmt.Errorf("deleting pod failed: %v", err)
	}

	klog.InfoS("pod deleted", "pod", podKey)

	return nil
}

// getRandomPodKey returns a random key based on the slice index.
func getRandomPodKey(keys []string) string {
	// rand.Intn panics if n <= 0
	if len(keys) == 1 {
		return keys[0]
	}
	// #nosec G404 - not generating any secret
	return keys[rand.Intn(len(keys)-1)]
}

// handleSignals shutdowns gracefully on SIGINT and SIGTEM signals.
func handleSignals() <-chan struct{} {
	sigCh := make(chan os.Signal, 1)
	stopCh := make(chan struct{})
	go func() {
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		close(stopCh)
	}()
	return stopCh
}
