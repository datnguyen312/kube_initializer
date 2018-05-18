package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ghodss/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/api/apps/v1beta1"
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
)

const (
	defaultAnnotation      = "initializer.suker200.guthub.com/nodeselector"
	defaultInitializerName = "nodeselector.initializer.kubernetes.io"
	defaultConfigmap       = "nodeselector-initializer"
	defaultNamespace       = "default"
)

var (
	annotation        string
	configmap         string
	initializerName   string
	namespace         string
	requireAnnotation bool
)

type initializerConfig struct {
	LOCAL_DNS struct {
		Enable bool `yaml:"enable"`
		NamespacePattern string `yaml:"namespacePattern"`
	} `yaml:"local_dns"`
	Developer struct {
		Enable bool `yaml:"enable"`
		NamespacePattern string `yaml:"namespacePattern"`
		NodeSelectorTerms	[]corev1.NodeSelectorTerm `yaml:"nodeSelectorTerms"`
		Ingress	struct {
			Class string `yaml:"class"`
		} `yaml:"ingress"`
		Service struct {
			Type  []corev1.ServiceType `yaml:"ClusterIP"`
		} `yaml:"service"`
	} `yaml:"developer"`
}

type Config struct {
	Containers []corev1.Container
	Volumes    []corev1.Volume
	UserConfig initializerConfig `yaml:"userConfig"`
}

func main() {
	flag.StringVar(&annotation, "annotation", defaultAnnotation, "The annotation to trigger initialization")
	flag.StringVar(&configmap, "configmap", defaultConfigmap, "The envoy initializer configuration configmap")
	flag.StringVar(&initializerName, "initializer-name", defaultInitializerName, "The initializer name")
	flag.StringVar(&namespace, "namespace", "default", "The configuration namespace")
	flag.BoolVar(&requireAnnotation, "require-annotation", false, "Require annotation for initialization")
	flag.Parse()

	log.Println("Starting the Kubernetes initializer...")
	log.Printf("Initializer name set to: %s", initializerName)

	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		log.Fatal(err)
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(configmap, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	c, err := configmapToConfig(cm)
	if err != nil {
		log.Fatal(err)
	}

	// log.Println(c)

	// Watch uninitialized Deployments in all namespaces.
	restClient := clientset.AppsV1beta1().RESTClient()
	watchlistDeployment := cache.NewListWatchFromClient(restClient, "deployments", corev1.NamespaceAll, fields.Everything())

	// Wrap the returned watchlistDeployment to workaround the inability to include
	// the `IncludeUninitialized` list option when setting up watch clients.
	includeUninitializedWatchlistDeployment := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.IncludeUninitialized = true
			return watchlistDeployment.List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.IncludeUninitialized = true
			return watchlistDeployment.Watch(options)
		},
	}

	includeUninitializedWatchlistService := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.IncludeUninitialized = true
			return clientset.CoreV1().Services(corev1.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.IncludeUninitialized = true
			return clientset.CoreV1().Services(corev1.NamespaceAll).Watch(options)
		},
	}

	includeUninitializedWatchlistIngress := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.IncludeUninitialized = true
			return clientset.Extensions().Ingresses(corev1.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.IncludeUninitialized = true
			return clientset.Extensions().Ingresses(corev1.NamespaceAll).Watch(options)
		},
	}

	resyncPeriod := 0 * time.Second

	_, controllerDeployment := cache.NewInformer(includeUninitializedWatchlistDeployment, &v1beta1.Deployment{}, resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				err := InitializeDeployment(obj.(*v1beta1.Deployment), c, clientset)
				if err != nil {
					log.Println(err)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				err := InitializeDeployment(new.(*v1beta1.Deployment), c, clientset)
				if err != nil {
					log.Println(err)
				}
			},
		},
	)

	_, controllerService := cache.NewInformer(includeUninitializedWatchlistService, &corev1.Service{}, resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				err := InitializeService(obj.(*corev1.Service), c, clientset)
				if err != nil {
					log.Println(err)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				err := InitializeService(new.(*corev1.Service), c, clientset)
				if err != nil {
					log.Println(err)
				}
			},
		},
	)

	_, controllerIngress := cache.NewInformer(includeUninitializedWatchlistIngress, &ext_v1beta1.Ingress{}, resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				err := InitializeIngress(obj.(*ext_v1beta1.Ingress), c, clientset)
				if err != nil {
					log.Println(err)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				err := InitializeIngress(new.(*ext_v1beta1.Ingress), c, clientset)
				if err != nil {
					log.Println(err)
				}
			},
		},
	)

	stop := make(chan struct{})
	go controllerDeployment.Run(stop)
	go controllerService.Run(stop)
	go controllerIngress.Run(stop)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	log.Println("Shutdown signal received, exiting...")
	close(stop)	
}


func configmapToConfig(configmap *corev1.ConfigMap) (*Config, error) {
	var c Config
	log.Println(configmap.Data["config"])
	err := yaml.Unmarshal([]byte(configmap.Data["config"]), &c)
	if err != nil {
		return nil, err
	}
	log.Println(c)
	return &c, nil
}