/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	cosiapi "github.com/container-object-storage-interface/api/apis/cosi.sigs.k8s.io/v1alpha1"
	cosiclient "github.com/container-object-storage-interface/api/clientset"
	cosiinformer "github.com/container-object-storage-interface/api/informers/externalversions"
	"golang.org/x/time/rate"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	glog "k8s.io/klog"
)

// annClass annotation represents the bucket class associated with a resource:
// - in Bucket it represents required class to match.
//   Only Buckets with the same class (i.e. annotation with the same
//   value) can be bound to the Bucket. In case no such bucket exists, the
//   controller will provision a new one using BucketClass instance with
//   the same name as the annotation value.
// - in Bucket it represents bucketclass to which the bucket belongs.
const annClass = "cosi.beta.kubernetes.io/bucket-class"

var (
	errStopProvision = errors.New("stop provisioning")
)

// ProvisionController is a controller that provisions Bucket in the backend storage system.
type ProvisionController struct {
	client     kubernetes.Interface
	cosiclient cosiclient.Interface

	// The name of the provisioner for which this controller dynamically
	// provisions buckets.
	provisionerName string

	// The provisioner the controller will use to provision buckets.
	bucket_provisioner BucketProvisioner

	// The provisioner the controller will use to provision buckets access.
	bucketaccess_provisioner BucketAccessProvisioner

	kubeVersion *utilversion.Version

	informerFactory cosiinformer.SharedInformerFactory

	bucketInformer cache.SharedIndexInformer
	bucketIndexer  cache.Indexer

	bucketAccessInformer cache.SharedInformer

	bucketClasses       cache.Store
	bucketAccessClasses cache.Store

	bucketQueue       workqueue.RateLimitingInterface
	bucketAccessQueue workqueue.RateLimitingInterface

	// Identity of this controller, generated at creation time and not persisted
	// across restarts. Useful only for debugging, for seeing the source of
	// events. controller.provisioner may have its own, different notion of
	// identity which may/may not persist across restarts
	id            string
	component     string
	eventRecorder record.EventRecorder

	resyncPeriod     time.Duration
	provisionTimeout time.Duration
	deletionTimeout  time.Duration

	rateLimiter               workqueue.RateLimiter
	exponentialBackOffOnError bool
	threadiness               int

	failedProvisionThreshold, failedDeleteThreshold int

	hasRun     bool
	hasRunLock *sync.Mutex

	bucketsInProgress        sync.Map
	bucketAccessesInProgress sync.Map

	bucketStore       cache.Store
	bucketAccessStore cache.Store
}

const (
	// DefaultResyncPeriod is used when option function ResyncPeriod is omitted
	DefaultResyncPeriod = 1 * time.Minute
	// DefaultThreadiness is used when option function Threadiness is omitted
	DefaultThreadiness = 4
	// DefaultExponentialBackOffOnError is used when option function ExponentialBackOffOnError is omitted
	DefaultExponentialBackOffOnError = true
	// DefaultCreateProvisionedBucketRetryCount is used when option function CreateProvisionedBucketRetryCount is omitted
	DefaultCreateProvisionedBucketRetryCount = 5
	// DefaultCreateProvisionedBucketnterval is used when option function CreateProvisionedBucketInterval is omitted
	DefaultCreateProvisionedBucketInterval = 10 * time.Second
	// DefaultFailedProvisionThreshold is used when option function FailedProvisionThreshold is omitted
	DefaultFailedProvisionThreshold = 15
	// DefaultFailedDeleteThreshold is used when option function FailedDeleteThreshold is omitted
	DefaultFailedDeleteThreshold = 15
	// DefaultLeaderElection is used when option function LeaderElection is omitted
	DefaultLeaderElection = true
	// DefaultLeaseDuration is used when option function LeaseDuration is omitted
	DefaultLeaseDuration = 15 * time.Second
	// DefaultRenewDeadline is used when option function RenewDeadline is omitted
	DefaultRenewDeadline = 10 * time.Second
	// DefaultRetryPeriod is used when option function RetryPeriod is omitted
	DefaultRetryPeriod = 2 * time.Second
	// DefaultMetricsPort is used when option function MetricsPort is omitted
	DefaultMetricsPort = 0
	// DefaultMetricsAddress is used when option function MetricsAddress is omitted
	DefaultMetricsAddress = "0.0.0.0"
	// DefaultMetricsPath is used when option function MetricsPath is omitted
	DefaultMetricsPath = "/metrics"
	// DefaultAddFinalizer is used when option function AddFinalizer is omitted
	DefaultAddFinalizer = false

	uidIndex = "uid"
)

var errRuntime = fmt.Errorf("cannot call option functions after controller has Run")

// ResyncPeriod is how often the controller relists buckets, buckets, & bucket
// classes. OnUpdate will be called even if nothing has changed, meaning failed
// operations may be retried on a bucket/bucket every resyncPeriod regardless of
// whether it changed. Defaults to 15 minutes.
func ResyncPeriod(resyncPeriod time.Duration) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.resyncPeriod = resyncPeriod
		return nil
	}
}

// Threadiness is the number of bucket and bucket workers each to launch.
// Defaults to 4.
func Threadiness(threadiness int) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.threadiness = threadiness
		return nil
	}
}

// RateLimiter is the workqueue.RateLimiter to use for the provisioning and
// deleting work queues. If set, ExponentialBackOffOnError is ignored.
func RateLimiter(rateLimiter workqueue.RateLimiter) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.rateLimiter = rateLimiter
		return nil
	}
}

// ExponentialBackOffOnError determines whether to exponentially back off from
// failures of Provision and Delete. Defaults to true.
func ExponentialBackOffOnError(exponentialBackOffOnError bool) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.exponentialBackOffOnError = exponentialBackOffOnError
		return nil
	}
}

// FailedProvisionThreshold is the threshold for max number of retries on
// failures of Provision. Set to 0 to retry indefinitely. Defaults to 15.
func FailedProvisionThreshold(failedProvisionThreshold int) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.failedProvisionThreshold = failedProvisionThreshold
		return nil
	}
}

// FailedDeleteThreshold is the threshold for max number of retries on failures
// of Delete. Set to 0 to retry indefinitely. Defaults to 15.
func FailedDeleteThreshold(failedDeleteThreshold int) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.failedDeleteThreshold = failedDeleteThreshold
		return nil
	}
}

// BucketInformer sets the informer to use for accessing Buckets.
// Defaults to using a internal informer.
func BucketInformer(informer cache.SharedIndexInformer) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.bucketInformer = informer
		return nil
	}
}

// BucketAccessInformer sets the informer to use for accessing Buckets.
// Defaults to using a internal informer.
func BucketAccessInformer(informer cache.SharedInformer) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.bucketAccessInformer = informer
		return nil
	}
}

// ProvisionTimeout sets the amount of time that provisioning a bucket may take.
// The default is unlimited.
func ProvisionTimeout(timeout time.Duration) func(*ProvisionController) error {
	return func(c *ProvisionController) error {
		if c.HasRun() {
			return errRuntime
		}
		c.provisionTimeout = timeout
		return nil
	}
}

// HasRun returns whether the controller has Run
func (ctrl *ProvisionController) HasRun() bool {
	ctrl.hasRunLock.Lock()
	defer ctrl.hasRunLock.Unlock()
	return ctrl.hasRun
}

// NewProvisionController creates a new provision controller using
// the given configuration parameters and with private (non-shared) informers.
func NewProvisionController(
	client kubernetes.Interface,
	cosiclient cosiclient.Interface,
	provisionerName string,
	bucket_provisioner BucketProvisioner,
	bucketaccess_provisioner BucketAccessProvisioner,
	kubeVersion string,
	options ...func(*ProvisionController) error,
) *ProvisionController {
	id, err := os.Hostname()
	if err != nil {
		glog.Fatalf("Error getting hostname: %v", err)
	}
	// add a unique identifier so that two processes on the same host don't accidentally both become active
	id = id + "_" + string(uuid.NewUUID())
	component := provisionerName + "_" + id

	v1.AddToScheme(scheme.Scheme)
	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(glog.Infof)
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: client.CoreV1().Events(v1.NamespaceAll)})
	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: component})

	controller := &ProvisionController{
		client:                    client,
		cosiclient:                cosiclient,
		provisionerName:           provisionerName,
		bucket_provisioner:        bucket_provisioner,
		bucketaccess_provisioner:  bucketaccess_provisioner,
		kubeVersion:               utilversion.MustParseSemantic(kubeVersion),
		id:                        id,
		component:                 component,
		eventRecorder:             eventRecorder,
		resyncPeriod:              DefaultResyncPeriod,
		exponentialBackOffOnError: DefaultExponentialBackOffOnError,
		threadiness:               DefaultThreadiness,
		failedProvisionThreshold:  DefaultFailedProvisionThreshold,
		failedDeleteThreshold:     DefaultFailedDeleteThreshold,
		hasRun:                    false,
		hasRunLock:                &sync.Mutex{},
	}

	for _, option := range options {
		err := option(controller)
		if err != nil {
			glog.Fatalf("Error processing controller options: %s", err)
		}
	}

	var rateLimiter workqueue.RateLimiter
	if controller.rateLimiter != nil {
		// rateLimiter set via parameter takes precedence
		rateLimiter = controller.rateLimiter
	} else if controller.exponentialBackOffOnError {
		rateLimiter = workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(15*time.Second, 1000*time.Second),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		)
	} else {
		rateLimiter = workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(15*time.Second, 15*time.Second),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		)
	}
	controller.bucketQueue = workqueue.NewNamedRateLimitingQueue(rateLimiter, "buckets")
	controller.bucketAccessQueue = workqueue.NewNamedRateLimitingQueue(rateLimiter, "bucketacceses")

	// ----------------------
	// Bucket

	bucketHandler := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { fmt.Println("Add B"); controller.enqueueBucket(obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { fmt.Println("Update B"); controller.enqueueBucket(newObj) },
		DeleteFunc: func(obj interface{}) {
			// NOOP. The bucket is either in bucketsInProgress and in the queue, so it will be processed as usual
			// or it's not in bucketsInProgress and then we don't care
		},
	}
	controller.informerFactory = cosiinformer.NewSharedInformerFactory(cosiclient, controller.resyncPeriod)

	if controller.bucketInformer != nil {
		controller.bucketInformer.AddEventHandlerWithResyncPeriod(bucketHandler, controller.resyncPeriod)
	} else {
		controller.bucketInformer = controller.informerFactory.Cosi().V1alpha1().Buckets().Informer()
		controller.bucketInformer.AddEventHandler(bucketHandler)
	}
	controller.bucketInformer.AddIndexers(cache.Indexers{uidIndex: func(obj interface{}) ([]string, error) {
		uid, err := getObjectUID(obj)
		if err != nil {
			return nil, err
		}
		return []string{uid}, nil
	}})
	controller.bucketIndexer = controller.bucketInformer.GetIndexer()
	controller.bucketStore = controller.bucketInformer.GetStore()

	// -----------------
	// BucketAccess

	bucketAccessHandler := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { fmt.Println("Add BA"); controller.enqueueBucketAccess(obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { fmt.Println("Update BA"); controller.enqueueBucketAccess(newObj) },
		//DeleteFunc: func(obj interface{}) { controller.forgetVolume(obj) },
	}

	if controller.bucketAccessInformer != nil {
		controller.bucketAccessInformer.AddEventHandlerWithResyncPeriod(bucketAccessHandler, controller.resyncPeriod)
	} else {
		controller.bucketAccessInformer = controller.informerFactory.Cosi().V1alpha1().BucketAccesses().Informer()
		controller.bucketAccessInformer.AddEventHandler(bucketAccessHandler)
	}
	controller.bucketAccessStore = controller.bucketAccessInformer.GetStore()

	return controller
}

func getObjectUID(obj interface{}) (string, error) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return "", fmt.Errorf("error decoding object, invalid type")
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			return "", fmt.Errorf("error decoding object tombstone, invalid type")
		}
	}
	return string(object.GetUID()), nil
}

// enqueueBucket takes an obj and converts it into UID that is then put onto bucket work queue.
func (ctrl *ProvisionController) enqueueBucket(obj interface{}) {
	uid, err := getObjectUID(obj)
	fmt.Println("enqueueBucket obj ", obj, " uid ", uid, " err ", err)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	if ctrl.bucketQueue.NumRequeues(uid) == 0 {
		ctrl.bucketQueue.Add(uid)
	}
}

// enqueueBucketAccess takes an obj and converts it into a namespace/name string which
// is then put onto the given work queue.
func (ctrl *ProvisionController) enqueueBucketAccess(obj interface{}) {
	var key string
	var err error
	fmt.Println("enqueueBucketAccess obj ", obj)
	if key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	// Re-Adding is harmless but try to add it to the queue only if it is not
	// already there, because if it is already there we *must* be retrying it
	//fmt.Println("Putting into bucketQueue ", key)
	if ctrl.bucketAccessQueue.NumRequeues(key) == 0 {
		ctrl.bucketAccessQueue.Add(key)
	}
}

// Run starts all of this controller's control loops
func (ctrl *ProvisionController) Run(stopCh <-chan struct{}) {
	run := func(ctx context.Context) {
		glog.Infof("Starting provisioner controller %s!", ctrl.component)
		defer utilruntime.HandleCrash()
		defer ctrl.bucketQueue.ShutDown()
		defer ctrl.bucketAccessQueue.ShutDown()

		stopCh := ctx.Done()
		ctrl.informerFactory.Start(stopCh)

		ctrl.hasRunLock.Lock()
		ctrl.hasRun = true
		ctrl.hasRunLock.Unlock()

		if !cache.WaitForCacheSync(stopCh, ctrl.bucketInformer.HasSynced, ctrl.bucketAccessInformer.HasSynced) {
			return
		}

		for i := 0; i < ctrl.threadiness; i++ {
			go wait.Until(ctrl.runBucketWorker, time.Second, ctx.Done())
			go wait.Until(ctrl.runBucketAccessWorker, time.Second, ctx.Done())
		}

		glog.Infof("Started provisioner controller %s!", ctrl.component)

		select {}
	}

	fmt.Println("Run controller")
	run(context.TODO())
	fmt.Println("Bye from Controller")
}

func (ctrl *ProvisionController) runBucketWorker() {
	for ctrl.processNextBucketWorkItem() {
	}
}

func (ctrl *ProvisionController) runBucketAccessWorker() {
	for ctrl.processNextBucketAccessWorkItem() {
	}
}

// processNextBucketWorkItem processes items from bucketQueue
func (ctrl *ProvisionController) processNextBucketWorkItem() bool {
	obj, shutdown := ctrl.bucketQueue.Get()

	if shutdown {
		return false
	}

	err := func() error {
		// Apply per-operation timeout.
		if ctrl.provisionTimeout != 0 {
			_, cancel := context.WithTimeout(context.TODO(), ctrl.provisionTimeout)
			defer cancel()
		}
		defer ctrl.bucketQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			ctrl.bucketQueue.Forget(obj)
			return fmt.Errorf("expected string in workqueue but got %#v", obj)
		}

		if err := ctrl.syncBucketHandler(context.TODO(), key); err != nil {
			if ctrl.failedProvisionThreshold == 0 {
				fmt.Sprintf("Retrying syncing bucket %q, failure %v", key, ctrl.bucketQueue.NumRequeues(obj))
				ctrl.bucketQueue.AddRateLimited(obj)
			} else if ctrl.bucketQueue.NumRequeues(obj) < ctrl.failedProvisionThreshold {
				fmt.Sprintf("Retrying syncing bucket %q because failures %v < threshold %v", key, ctrl.bucketQueue.NumRequeues(obj), ctrl.failedProvisionThreshold)
				ctrl.bucketQueue.AddRateLimited(obj)
			} else {
				fmt.Sprintf("Giving up syncing bucket %q because failures %v >= threshold %v", key, ctrl.bucketQueue.NumRequeues(obj), ctrl.failedProvisionThreshold)
				glog.V(2).Infof("Removing Bucket %s from buckets in progress", key)
				// Done but do not Forget: it will not be in the queue but NumRequeues
				// will be saved until the obj is deleted from kubernetes
			}
			return fmt.Errorf("error syncing bucket %q: %s", key, err.Error())
		}
		ctrl.bucketQueue.Forget(obj)
		return nil
	}()

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// processNextBucketAccessWorkItem processes items from bucketAccessQueue
func (ctrl *ProvisionController) processNextBucketAccessWorkItem() bool {
	obj, shutdown := ctrl.bucketAccessQueue.Get()

	if shutdown {
		return false
	}

	err := func() error {
		// Apply per-operation timeout.
		if ctrl.deletionTimeout != 0 {
			_, cancel := context.WithTimeout(context.TODO(), ctrl.deletionTimeout)
			defer cancel()
		}
		defer ctrl.bucketAccessQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			ctrl.bucketAccessQueue.Forget(obj)
			return fmt.Errorf("expected string in workqueue but got %#v", obj)
		}

		if err := ctrl.syncBucketAccessHandler(context.TODO(), key); err != nil {
			if ctrl.failedDeleteThreshold == 0 {
				glog.Warningf("Retrying syncing bucket %q, failure %v", key, ctrl.bucketAccessQueue.NumRequeues(obj))
				ctrl.bucketAccessQueue.AddRateLimited(obj)
			} else if ctrl.bucketAccessQueue.NumRequeues(obj) < ctrl.failedDeleteThreshold {
				glog.Warningf("Retrying syncing bucket %q because failures %v < threshold %v", key, ctrl.bucketAccessQueue.NumRequeues(obj), ctrl.failedDeleteThreshold)
				ctrl.bucketAccessQueue.AddRateLimited(obj)
			} else {
				glog.Errorf("Giving up syncing bucket %q because failures %v >= threshold %v", key, ctrl.bucketAccessQueue.NumRequeues(obj), ctrl.failedDeleteThreshold)
				// Done but do not Forget: it will not be in the queue but NumRequeues
				// will be saved until the obj is deleted from kubernetes
			}
			return fmt.Errorf("error syncing bucket %q: %s", key, err.Error())
		}

		ctrl.bucketAccessQueue.Forget(obj)
		return nil
	}()

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncBucketHandler gets the bucket from informer's cache then calls syncBucket. A non-nil error triggers requeuing of the bucket.
func (ctrl *ProvisionController) syncBucketHandler(ctx context.Context, key string) error {
	objs, err := ctrl.bucketIndexer.ByIndex(uidIndex, key)
	if err != nil {
		return err
	}
	var bucketObj interface{}
	if len(objs) > 0 {
		bucketObj = objs[0]
	} else {
		obj, found := ctrl.bucketsInProgress.Load(key)
		if !found {
			utilruntime.HandleError(fmt.Errorf("bucket %q in work queue no longer exists", key))
			return nil
		}
		bucketObj = obj
	}
	return ctrl.syncBucket(ctx, bucketObj)
}

// syncBucketAccessHandler gets the bucket from informer's cache then calls syncbucket
func (ctrl *ProvisionController) syncBucketAccessHandler(ctx context.Context, key string) error {
	bucketAccessObj, exists, err := ctrl.bucketAccessStore.GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		utilruntime.HandleError(fmt.Errorf("bucketAccess %q in work queue no longer exists", key))
		return nil
	}

	return ctrl.syncBucketAccess(ctx, bucketAccessObj)
}

// syncBucket checks if the bucket should have a bucket provisioned for it and
// provisions one if so. Returns an error if the bucket is to be requeued.
func (ctrl *ProvisionController) syncBucket(ctx context.Context, obj interface{}) error {
	bucket, ok := obj.(*cosiapi.Bucket)
	if !ok {
		return fmt.Errorf("expected bucket but got %+v", obj)
	}

	should, err := ctrl.shouldProvisionBucket(ctx, bucket)
	if err != nil {
		return err
	} else if should {

		status, err := ctrl.provisionBucketOperation(ctx, bucket)
		if err == nil || status == ProvisioningFinished {
			// Provisioning is 100% finished / not in progress.
			switch err {
			case nil:
				glog.V(5).Infof("Bucket processing succeeded, removing bucket %s from buckets in progress", bucket.UID)
			case errStopProvision:
				glog.V(5).Infof("Stop provisioning, removing bucket %s from buckets in progress", bucket.UID)
				// Our caller would requeue if we pass on this special error; return nil instead.
				err = nil
			default:
				glog.V(2).Infof("Final error received, removing buckerRequest %s from buckets in progress", bucket.UID)
			}
			ctrl.bucketsInProgress.Delete(string(bucket.UID))
			return err
		}
		if status == ProvisioningInBackground {
			// Provisioning is in progress in background.
			glog.V(2).Infof("Temporary error received, adding bucket %s to buckets in progress", bucket.UID)
			ctrl.bucketsInProgress.Store(string(bucket.UID), bucket)
		} else {
			// status == ProvisioningNoChange.
			// Don't change bucketsInProgress:
			// - the bucket is already there if previous status was ProvisioningInBackground.
			// - the bucket is not there if if previous status was ProvisioningFinished.
		}
		return err
	}
	return nil
}

// syncBucketAccess checks if the bucketAccess should be deleted and deletes if so
func (ctrl *ProvisionController) syncBucketAccess(ctx context.Context, obj interface{}) error {
	bucketAccess, ok := obj.(*cosiapi.BucketAccess)
	if !ok || bucketAccess == nil {
		return fmt.Errorf("expected bucket but got %+v", obj)
	}
	fmt.Println(bucketAccess)

	should, err := ctrl.shouldProvisionBucketAccess(ctx, bucketAccess)
	if err != nil {
		return err
	} else if should {

		status, err := ctrl.provisionBucketAccessOperation(ctx, bucketAccess)
		if err == nil || status == ProvisioningFinished {
			// Provisioning is 100% finished / not in progress.
			switch err {
			case nil:
				glog.V(5).Infof("BucketAccess processing succeeded, removing bucketAccess %s from bucketAccesss in progress", bucketAccess.UID)
			case errStopProvision:
				glog.V(5).Infof("Stop provisioning, removing bucketAccess %s from bucketAccesss in progress", bucketAccess.UID)
				// Our caller would requeue if we pass on this special error; return nil instead.
				err = nil
			default:
				glog.V(2).Infof("Final error received, removing bucketAccess %s from bucketAccesss in progress", bucketAccess.UID)
			}
			ctrl.bucketAccessesInProgress.Delete(string(bucketAccess.UID))
			return err
		}
		if status == ProvisioningInBackground {
			// Provisioning is in progress in background.
			glog.V(2).Infof("Temporary error received, adding bucketAccess %s to bucketAccesss in progress", bucketAccess.UID)
			ctrl.bucketAccessesInProgress.Store(string(bucketAccess.UID), bucketAccess)
		} else {
			// status == ProvisioningNoChange.
			// Don't change bucketAccesssInProgress:
			// - the bucketAccess is already there if previous status was ProvisioningInBackground.
			// - the bucketAccess is not there if if previous status was ProvisioningFinished.
		}
		return err
	}
	return nil

	return nil
}

// knownProvisioner checks if provisioner name has been
// configured to provision buckets for
func (ctrl *ProvisionController) knownProvisioner(provisioner string) bool {
	if provisioner == ctrl.provisionerName {
		return true
	}
	return false
}

// shouldProvision returns whether a bucket should have a bucket provisioned for
// it, i.e. whether a Provision is "desired"
func (ctrl *ProvisionController) shouldProvisionBucket(ctx context.Context, bucket *cosiapi.Bucket) (bool, error) {
	if bucket.Name == "" {
		return false, nil
	}

	if qualifier, ok := ctrl.bucket_provisioner.(Qualifier); ok {
		if !qualifier.ShouldProvisionBucket(ctx, bucket) {
			return false, nil
		}
	}

	return true, nil
}

// shouldProvision returns whether a bucketAccess should have a bucket access provisioned for
// it, i.e. whether a Provision is "desired"
func (ctrl *ProvisionController) shouldProvisionBucketAccess(ctx context.Context, bucketAccess *cosiapi.BucketAccess) (bool, error) {
	if bucketAccess.Name != "" {
		return false, nil
	}

	if qualifier, ok := ctrl.bucketaccess_provisioner.(Qualifier); ok {
		if !qualifier.ShouldProvisionBucketAccess(ctx, bucketAccess) {
			return false, nil
		}
	}
	return true, nil
}

// provisionBucketOperation attempts to provision a bucket for the given bucket.
// Returns nil error only when the bucket was provisioned (in which case it also returns ProvisioningFinished),
// a normal error when the bucket was not provisioned and provisioning should be retried (requeue the bucket),
// or the special errStopProvision when provisioning was impossible and no further attempts to provision should be tried.
func (ctrl *ProvisionController) provisionBucketOperation(ctx context.Context, bucket *cosiapi.Bucket) (ProvisioningState, error) {
	// Most code here is identical to that found in controller.go of kube's  controller...
	bucketClassName := bucket.Spec.BucketClassName
	operation := fmt.Sprintf("provision %q class %q", bucket.Name, bucketClassName)
	glog.Info(logOperation(operation, "started"))

	// TODO simple InClusterConfig should be used
	kubeconfigEnv := os.Getenv("KUBECONFIG")

	kubeconfig := ""
	var config *rest.Config
	var err error
	if kubeconfigEnv != "" {
		glog.Infof("Found KUBECONFIG environment variable set, using that..")
		kubeconfig = kubeconfigEnv
	}

	if kubeconfig != "" {
		glog.Infof("Either master or kubeconfig specified. building kube config from that..")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		glog.Infof("Building kube configs for running in cluster...")
		config, err = rest.InClusterConfig()
	}

	/*
	        config, err := rest.InClusterConfig()
	   	if err != nil {
	   		glog.Fatalf("Failed to create config: %v", err)
	   	}
	*/

	bucketClass, err := cosiclient.NewForConfigOrDie(config).CosiV1alpha1().BucketClasses().Get(ctx, bucketClassName, metav1.GetOptions{})
	if bucketClass == nil {
		// bucket has been already provisioned, nothing to do.
		glog.Info(logOperation(operation, "bucketClass %q does not exist, skipping", err))
		return ProvisioningFinished, errStopProvision
	}

	fmt.Println("BucketClass ", bucketClass)

	options := BucketProvisionOptions{
		// TODO reword provisioner options, we may not need some of these
		//Bucketequest:  bucketrequest,
		Bucket:      bucket,
		BucketClass: bucketClass,
	}

	ctrl.eventRecorder.Event(bucket, v1.EventTypeNormal, "Provisioning", fmt.Sprintf("External provisioner is provisioning bucket for bucket %q", bucket.Name))

	bucket, result, err := ctrl.bucket_provisioner.CreateBucket(ctx, options)
	if err != nil {
		if ierr, ok := err.(*IgnoredError); ok {
			// Provision ignored, do nothing and hope another provisioner will provision it.
			glog.Info(logOperation(operation, "bucket provision ignored: %v", ierr))
			return ProvisioningFinished, errStopProvision
		}
		err = fmt.Errorf("failed to provision bucket with BucketClass %v: %v", bucketClass, err)
		ctrl.eventRecorder.Event(bucket, v1.EventTypeWarning, "ProvisioningFailed", err.Error())

		// ProvisioningReschedule shouldn't have been returned for buckets without selected node,
		// but if we get it anyway, then treat it like ProvisioningFinished because we cannot
		// reschedule.
		if result == ProvisioningReschedule {
			result = ProvisioningFinished
		}
		return result, err
	}

	glog.Info(logOperation(operation, "bucket %q provisioned", bucket))

	// TODO update bucket status and persist.
	glog.Info(logOperation(operation, "succeeded"))

	return ProvisioningFinished, nil
}

// provisionBucketOperation attempts to provision a bucket for the given bucket.
// Returns nil error only when the bucket was provisioned (in which case it also returns ProvisioningFinished),
// a normal error when the bucket was not provisioned and provisioning should be retried (requeue the bucket),
// or the special errStopProvision when provisioning was impossible and no further attempts to provision should be tried.
func (ctrl *ProvisionController) provisionBucketAccessOperation(ctx context.Context, bucketAccess *cosiapi.BucketAccess) (ProvisioningState, error) {
	return ProvisioningNoChange, nil
}

func logOperation(operation, format string, a ...interface{}) string {
	return fmt.Sprintf(fmt.Sprintf("%s: %s", operation, format), a...)
}
