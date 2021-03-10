// Copyright 2018-2020 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package watchers

import (
	"context"
	"sync"
	"time"

	"github.com/cilium/cilium/pkg/k8s"
	"github.com/cilium/cilium/pkg/k8s/informer"
	slim_corev1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
	slim_discover_v1beta1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/discovery/v1beta1"
	"github.com/cilium/cilium/pkg/k8s/utils"
	"github.com/cilium/cilium/pkg/k8s/watchers/subscriber"
	"github.com/cilium/cilium/pkg/kvstore/store"
	"github.com/cilium/cilium/pkg/loadbalancer"
	"github.com/cilium/cilium/pkg/lock"
	"github.com/cilium/cilium/pkg/logging/logfields"
	"github.com/cilium/cilium/pkg/metrics"
	serviceStore "github.com/cilium/cilium/pkg/service/store"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var (
	K8sSvcCache = k8s.NewServiceCache(nil)

	// k8sSvcCacheSynced is used do signalize when all services are synced with
	// k8s.
	k8sSvcCacheSynced = make(chan struct{})
	kvs               *store.SharedStore
	sharedOnly        bool

	serviceSubscribers = subscriber.NewService()
)

func k8sServiceHandler(clusterName string) {
	serviceHandler := func(event k8s.ServiceEvent) {
		defer event.SWG.Done()

		svc := k8s.NewClusterService(event.ID, event.Service, event.Endpoints)
		svc.Cluster = clusterName

		log.WithFields(logrus.Fields{
			logfields.K8sSvcName:   event.ID.Name,
			logfields.K8sNamespace: event.ID.Namespace,
			"action":               event.Action.String(),
			"service":              event.Service.String(),
			"endpoints":            event.Endpoints.String(),
			"shared":               event.Service.Shared,
		}).Debug("Kubernetes service definition changed")

		if sharedOnly && !event.Service.Shared {
			// The annotation may have been added, delete an eventual existing service
			kvs.DeleteLocalKey(context.TODO(), &svc)
			return
		}

		switch event.Action {
		case k8s.UpdateService:
			kvs.UpdateLocalKeySync(context.TODO(), &svc)

		case k8s.DeleteService:
			kvs.DeleteLocalKey(context.TODO(), &svc)
		}
	}
	for {
		event, ok := <-K8sSvcCache.Events
		if !ok {
			return
		}

		serviceHandler(event)
	}
}

// ServiceSyncConfiguration is the required configuration for StartSynchronizingServices
type ServiceSyncConfiguration interface {
	// LocalClusterName must return the local cluster name
	LocalClusterName() string

	utils.ServiceConfiguration
}

// StartSynchronizingServices starts a controller for synchronizing services from k8s to kvstore
// 'shared' specifies whether only shared services are synchronized. If 'false' then all services
// will be synchronized. For clustermesh we only need to synchronize shared services, while for
// VM support we need to sync all the services.
func StartSynchronizingServices(shared bool, cfg ServiceSyncConfiguration) {
	log.Info("Starting to synchronize k8s services to kvstore")
	sharedOnly = shared

	serviceOptsModifier, err := utils.GetServiceListOptionsModifier(cfg)
	if err != nil {
		log.WithError(err).Fatal("Error creating service option modifier")
	}

	readyChan := make(chan struct{}, 0)

	go func() {
		store, err := store.JoinSharedStore(store.Configuration{
			Prefix: serviceStore.ServiceStorePrefix,
			KeyCreator: func() store.Key {
				return &serviceStore.ClusterService{}
			},
			SynchronizationInterval: 5 * time.Minute,
		})

		if err != nil {
			log.WithError(err).Fatal("Unable to join kvstore store to announce services")
		}

		kvs = store
		close(readyChan)
	}()

	swgSvcs := lock.NewStoppableWaitGroup()
	swgEps := lock.NewStoppableWaitGroup()

	serviceSubscribers.Register(newServiceCacheSubscriber(swgSvcs, swgEps))

	// Watch for v1.Service changes and push changes into ServiceCache
	_, svcController := informer.NewInformer(
		cache.NewFilteredListWatchFromClient(k8s.WatcherClient().CoreV1().RESTClient(),
			"services", v1.NamespaceAll, serviceOptsModifier),
		&slim_corev1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				if k8sSvc := k8s.ObjToV1Services(obj); k8sSvc != nil {
					serviceSubscribers.NotifyAdd(k8sSvc)
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				if oldk8sSvc := k8s.ObjToV1Services(oldObj); oldk8sSvc != nil {
					if newk8sSvc := k8s.ObjToV1Services(newObj); newk8sSvc != nil {
						if oldk8sSvc.DeepEqual(newk8sSvc) {
							return
						}
						serviceSubscribers.NotifyUpdate(oldk8sSvc, newk8sSvc)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				k8sSvc := k8s.ObjToV1Services(obj)
				if k8sSvc == nil {
					return
				}
				serviceSubscribers.NotifyDelete(k8sSvc)
			},
		},
		nil,
	)

	go svcController.Run(wait.NeverStop)

	var (
		endpointController cache.Controller
	)

	// We only enable either "Endpoints" or "EndpointSlice"
	switch {
	case k8s.SupportsEndpointSlice():
		var endpointSliceEnabled bool
		endpointController, endpointSliceEnabled = endpointSlicesInit(k8s.WatcherClient(), swgEps)
		// the cluster has endpoint slices so we should not check for v1.Endpoints
		if endpointSliceEnabled {
			break
		}
		fallthrough
	default:
		endpointController = endpointsInit(k8s.WatcherClient(), swgEps, serviceOptsModifier)
		go endpointController.Run(wait.NeverStop)
	}

	go func() {
		cache.WaitForCacheSync(wait.NeverStop, svcController.HasSynced)
		swgSvcs.Stop()
		swgSvcs.Wait()
		close(k8sSvcCacheSynced)

		cache.WaitForCacheSync(wait.NeverStop, endpointController.HasSynced)
	}()

	go func() {
		<-readyChan
		log.Info("Starting to synchronize Kubernetes services to kvstore")
		k8sServiceHandler(cfg.LocalClusterName())
	}()
}

func endpointsInit(k8sClient kubernetes.Interface, swgEps *lock.StoppableWaitGroup, optsModifier func(*v1meta.ListOptions)) cache.Controller {
	epOptsModifier := func(options *v1meta.ListOptions) {
		// Don't get any events from kubernetes endpoints.
		options.FieldSelector = fields.ParseSelectorOrDie("metadata.name!=kube-scheduler,metadata.name!=kube-controller-manager").String()
		optsModifier(options)
	}

	// Watch for v1.Endpoints changes and push changes into ServiceCache
	_, endpointController := informer.NewInformer(
		cache.NewFilteredListWatchFromClient(k8sClient.CoreV1().RESTClient(),
			"endpoints", v1.NamespaceAll,
			epOptsModifier,
		),
		&slim_corev1.Endpoints{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				if k8sEP := k8s.ObjToV1Endpoints(obj); k8sEP != nil {
					K8sSvcCache.UpdateEndpoints(k8sEP, swgEps)
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				if oldk8sEP := k8s.ObjToV1Endpoints(oldObj); oldk8sEP != nil {
					if newk8sEP := k8s.ObjToV1Endpoints(newObj); newk8sEP != nil {
						if oldk8sEP.DeepEqual(newk8sEP) {
							return
						}
						K8sSvcCache.UpdateEndpoints(newk8sEP, swgEps)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				k8sEP := k8s.ObjToV1Endpoints(obj)
				if k8sEP == nil {
					return
				}
				K8sSvcCache.DeleteEndpoints(k8sEP, swgEps)
			},
		},
		nil,
	)
	return endpointController
}

func endpointSlicesInit(k8sClient kubernetes.Interface, swgEps *lock.StoppableWaitGroup) (cache.Controller, bool) {
	var (
		hasEndpointSlices = make(chan struct{})
		once              sync.Once
	)

	_, endpointController := informer.NewInformer(
		cache.NewListWatchFromClient(k8sClient.DiscoveryV1beta1().RESTClient(),
			"endpointslices", v1.NamespaceAll, fields.Everything()),
		&slim_discover_v1beta1.EndpointSlice{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				once.Do(func() {
					// signalize that we have received an endpoint slice
					// so it means the cluster has endpoint slices enabled.
					close(hasEndpointSlices)
				})
				metrics.EventTSK8s.SetToCurrentTime()
				if k8sEP := k8s.ObjToV1EndpointSlice(obj); k8sEP != nil {
					K8sSvcCache.UpdateEndpointSlices(k8sEP, swgEps)
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				if oldk8sEP := k8s.ObjToV1EndpointSlice(oldObj); oldk8sEP != nil {
					if newk8sEP := k8s.ObjToV1EndpointSlice(newObj); newk8sEP != nil {
						if oldk8sEP.DeepEqual(newk8sEP) {
							return
						}
						K8sSvcCache.UpdateEndpointSlices(newk8sEP, swgEps)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				metrics.EventTSK8s.SetToCurrentTime()
				k8sEP := k8s.ObjToV1EndpointSlice(obj)
				if k8sEP == nil {
					return
				}
				K8sSvcCache.DeleteEndpointSlices(k8sEP, swgEps)
			},
		},
		nil,
	)
	ecr := make(chan struct{})
	go endpointController.Run(ecr)

	if k8s.HasEndpointSlice(hasEndpointSlices, endpointController) {
		return endpointController, true
	}

	// K8s is not running with endpoint slices enabled, stop the endpoint slice
	// controller to avoid watching for unnecessary stuff in k8s.
	close(ecr)

	return nil, false
}

// ServiceGetter is a wrapper for 2 k8sCaches, its intention is for
// `shortCutK8sCache` to be used until `k8sSvcCacheSynced` is closed, for which
// `k8sCache` is started to be used.
type ServiceGetter struct {
	shortCutK8sCache k8s.ServiceIPGetter
	k8sCache         k8s.ServiceIPGetter
}

// NewServiceGetter returns a new ServiceGetter holding 2 k8sCaches
func NewServiceGetter(sc *k8s.ServiceCache) *ServiceGetter {
	return &ServiceGetter{
		shortCutK8sCache: sc,
		k8sCache:         &K8sSvcCache,
	}
}

// GetServiceIP returns the result of GetServiceIP for `s.shortCutK8sCache`
// until `k8sSvcCacheSynced` is closed. This is helpful as we can have a
// shortcut of `s.k8sCache` since we can pre-populate `s.shortCutK8sCache` with
// the entries that we need until `s.k8sCache` is synchronized with kubernetes.
func (s *ServiceGetter) GetServiceIP(svcID k8s.ServiceID) *loadbalancer.L3n4Addr {
	select {
	case <-k8sSvcCacheSynced:
		return s.k8sCache.GetServiceIP(svcID)
	default:
		return s.shortCutK8sCache.GetServiceIP(svcID)
	}
}
