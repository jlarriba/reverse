package main

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// Controller is the struct that holds the data needed for the Kubernetes Controller to run
type Controller struct {
	indexer   cache.Indexer
	queue     workqueue.RateLimitingInterface
	informer  cache.Controller
	clientset *kubernetes.Clientset
}

// Event holds the data that is put in the Queue ready for processing
type Event struct {
	key  string
	kind string
}

// NewController creates a new instance of Controller
func NewController(queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller, clientset *kubernetes.Clientset) *Controller {
	return &Controller{
		informer:  informer,
		indexer:   indexer,
		queue:     queue,
		clientset: clientset,
	}
}

// NewEvent creates a new instance of Event
func NewEvent(key string, kind string) *Event {
	return &Event{
		key:  key,
		kind: kind,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	event, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(event)

	// Invoke the method containing the business logic depending on the triggering event
	if event.(*Event).kind == "delete" {
		err := c.deleteReversedNamespace(event.(*Event).key)
		c.handleErr(err, event)
	} else {
		err := c.createReversedNamespace(event.(*Event).key)
		c.handleErr(err, event)
	}
	return true
}

// reverse is a very classic method in Go, as the stdlib does not provide a way to reverse strings
func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// createReversedNamespace creates a new namespace with the name reversed each time a namespace is created
func (c *Controller) createReversedNamespace(key string) error {
	_, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	if exists {
		nsSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: reverse(key)}}
		namespace, _ := c.clientset.CoreV1().Namespaces().Create(nsSpec)
		fmt.Printf("Namespace %v created because %v was created\n", namespace.Name, key)
	}
	return nil
}

// createReversedNamespace deletes a new namespace with the name reversed each time a namespace is deleted
func (c *Controller) deleteReversedNamespace(key string) error {
	_, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	// As the event fires when the namespace does not exist anymore, we need to be sure that has been deleted
	if !exists {
		c.clientset.CoreV1().Namespaces().Delete(reverse(key), nil)
		fmt.Printf("Namespace %v deleted because %v was deleted\n", reverse(key), key)
	}
	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, event interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(event)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(event) < 5 {
		klog.Infof("Error syncing ns %v: %v", event, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(event)
		return
	}

	c.queue.Forget(event)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	klog.Infof("Dropping ns %q out of the queue: %v", event, err)
}

// Run launches the controller
func (c *Controller) Run(threadiness int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	klog.Info("Starting Namespace controller")

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	klog.Info("Stopping Namespace controller")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}
