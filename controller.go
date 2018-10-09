package main

import (
	"fmt"
	"github.com/golang/glog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	eventsinformers "k8s.io/client-go/informers/events/v1beta1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	eventslisters "k8s.io/client-go/listers/events/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func pCount(event, msg, pod, node string, err error) {
	if err == nil {
		err = fmt.Errorf("")
	}
	pvwatchCount.With(prometheus.Labels{
		"pod": pod, "event": event, "msg": msg, "err": err.Error(), "node": node,
	}).Add(1)
}

const (
	eventNotFound = "event_not_found"
	eventMismatch = "event_mismatch"
	podNotFound   = "pod_not_found"
	podDelete     = "unable_to_delete_pod"
	podPhase      = "pod_not_pending"
	podCache      = "pod_in_delete_cache"
)

type Controller struct {
	kubeclientset kubernetes.Interface

	podLister   corelisters.PodLister
	podSynced   cache.InformerSynced
	eventLister eventslisters.EventLister
	eventSynced cache.InformerSynced
	deleteCache Cache

	workqueue workqueue.RateLimitingInterface
}

func NewController(
	kubeclientset kubernetes.Interface,
	podInformer coreinformers.PodInformer,
	eventsInformer eventsinformers.EventInformer) *Controller {

	controller := &Controller{
		kubeclientset: kubeclientset,
		podLister:     podInformer.Lister(),
		podSynced:     podInformer.Informer().HasSynced,
		eventLister:   eventsInformer.Lister(),
		eventSynced:   eventsInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Events"),
		deleteCache:   NewCache(1 * time.Minute),
	}

	eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueEvent,
		UpdateFunc: func(old, new interface{}) { controller.enqueueEvent(new) },
	})

	return controller
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()
	glog.Info("Starting pvwatch controller")
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.podSynced, c.eventSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	glog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")
	return nil
}

func (c *Controller) runWorker() {
	var err error
	var shutdown bool
	for !shutdown {
		shutdown, err = c.processNextWorkItem()
		if err != nil {
			runtime.HandleError(err)
		}
	}
}

func (c *Controller) processNextWorkItem() (bool, error) {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return shutdown, nil
	}
	defer c.workqueue.Done(obj)
	key, ok := obj.(string)
	if !ok {
		c.workqueue.Forget(obj)
		return false, fmt.Errorf("expected string in workqueue but got %#v", obj)
	}
	if err := c.deletePod(key); err != nil {
		return false, fmt.Errorf("error syncing '%s': %s", key, err.Error())
	}
	c.workqueue.Forget(obj)
	glog.Infof("Successfully synced '%s'", key)
	return false, nil
}

func (c *Controller) deletePod(eventCacheKey string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(eventCacheKey)
	if err != nil {
		glog.Errorf("Cache failed to find eventCacheKey %s", eventCacheKey)
		return err
	}
	event, err := c.eventLister.Events(namespace).Get(name)
	if err != nil {
		pCount(eventCacheKey, eventNotFound, "", "", err)
		glog.Infof("Event %s/%s not found", namespace, name)
		return nil
	}
	if !devicePathRe.MatchString(event.Note) {
		pCount(eventCacheKey, eventMismatch, "", "", fmt.Errorf(event.Note))
		glog.Infof("Event %s/%s note %s doesn't match", event.Namespace, event.Name, event.Note)
		return nil
	}
	reg := event.Regarding
	pod, err := c.podLister.Pods(reg.Namespace).Get(reg.Name)
	if err != nil {
		p := reg.Namespace + "/" + reg.Name
		pCount(eventCacheKey, podNotFound, p, "", err)
		glog.Infof("Pod %s not found", p)
		return nil
	}
	p := pod.Namespace + "/" + pod.Name
	n := pod.Spec.NodeName
	if pod.Status.Phase == corev1.PodPending && !c.deleteCache.Contains(p) {
		glog.Infof("Deleting pod %v", p)
		c.deleteCache.Put(p)
		if err = c.kubeclientset.CoreV1().Pods(pod.Namespace).Delete(pod.Name, &metav1.DeleteOptions{}); err != nil {
			glog.Errorf("Unable to delete pod %s, %s", p, err)
			pCount(eventCacheKey, podDelete, p, n, err)
			return err
		}
	} else {
		if pod.Status.Phase != corev1.PodPending {
			glog.Infof("Pod %s not in Pending phase, %s", p, pod.Status.Phase)
			pCount(eventCacheKey, podPhase, p, n, nil)
		} else {
			glog.Infof("Pod %s delete cache, %s", p, pod.Status.Phase)
			pCount(eventCacheKey, podCache, p, n, nil)
		}
		return nil
	}
	pCount(eventCacheKey, "ok", p, n, nil)
	return nil
}

// enqueueEvent takes the event and checks for known cinder status error `emptyPath`
// and add it to the queue for further processing
func (c *Controller) enqueueEvent(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			runtime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	glog.V(4).Infof("Processing object: %s", object.GetName())
	if key, err := cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	} else {
		c.workqueue.AddRateLimited(key)
	}
}
