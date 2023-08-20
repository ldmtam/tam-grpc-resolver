package tam_grpc_resolver

import (
	"context"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc/resolver"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// init function needs for auto-register in resolvers registry
func init() {
	resolver.Register(&builder{})
}

type tamResolver struct {
	appName   string
	namespace string
	port      string
	label     string

	ctx    context.Context
	cancel context.CancelFunc

	wg        sync.WaitGroup
	cc        resolver.ClientConn
	clientset *kubernetes.Clientset
}

func (r *tamResolver) ResolveNow(resolver.ResolveNowOptions) {}

// Close closes the resolver.
func (r *tamResolver) Close() {
	r.cancel()
	r.wg.Wait()
}

func (r *tamResolver) watcher() {
	labelOptions := informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
		opts.LabelSelector = r.label
	})

	// create a new instance of sharedInformerFactory
	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		r.clientset,
		1*time.Minute,
		informers.WithNamespace(r.namespace),
		labelOptions,
	)

	// using this factory create an informer for `Pods` resources
	podsInformer := informerFactory.Core().V1().Pods()

	// adds an event handler to the shared informer
	podsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if err := r.updatePodAddrs(); err != nil {
				log.Fatalf("Update pods IP with ADD event failed: %v", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if err := r.updatePodAddrs(); err != nil {
				log.Fatalf("Update pods IP with UPDATE event failed: %v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if err := r.updatePodAddrs(); err != nil {
				log.Fatalf("Update pods IP with DELETE event failed: %v", err)
			}
		},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)

	// starts the shared informers that have been created by the factory
	informerFactory.Start(stopCh)

	// wait for the initial synchronization of the local cache
	if !cache.WaitForCacheSync(stopCh, podsInformer.Informer().HasSynced) {
		log.Fatalf("failed to sync informer cache")
	}

	<-r.ctx.Done()
}

func (r *tamResolver) updatePodAddrs() error {
	podList, err := r.clientset.CoreV1().Pods(r.namespace).List(
		r.ctx,
		metav1.ListOptions{
			LabelSelector: r.label,
		},
	)
	if err != nil {
		return err
	}

	podAddrs := make([]resolver.Address, 0, len(podList.Items))
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			addr := pod.Status.PodIP + ":" + r.port
			podAddrs = append(podAddrs, resolver.Address{Addr: addr})
		}
	}

	state := resolver.State{Addresses: podAddrs}
	return r.cc.UpdateState(state)
}
