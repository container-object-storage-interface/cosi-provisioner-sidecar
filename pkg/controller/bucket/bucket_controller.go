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

package bucket

import (
	"context"
	"fmt"
	"strings"
	"time"

	utilversion "k8s.io/apimachinery/pkg/util/version"

	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"

	"github.com/container-object-storage-interface/api/apis/objectstorage.k8s.io/v1alpha1"
	bucketclientset "github.com/container-object-storage-interface/api/clientset"
	"github.com/container-object-storage-interface/api/controller"
	osspec "github.com/container-object-storage-interface/spec"

	"k8s.io/klog"

	"golang.org/x/time/rate"
)

// bucketListener manages Bucket objects
type bucketListener struct {
	kubeClient        kubeclientset.Interface
	bucketClient      bucketclientset.Interface
	provisionerClient osspec.ProvisionerClient

	// The name of the provisioner for which this controller dynamically
	// provisions buckets.
	provisionerName string
	kubeVersion     *utilversion.Version
}

func NewBucketController(provisionerName string, client osspec.ProvisionerClient) (*controller.ObjectStorageController, error) {
	rateLimit := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Second, 60*time.Minute),
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)

	identity := fmt.Sprintf("object-storage-sidecar-%s", provisionerName)
	bc, err := controller.NewObjectStorageController(identity, "bucket-controller", 5, rateLimit)
	if err != nil {
		return nil, err
	}

	bl := bucketListener{
		provisionerName:   provisionerName,
		provisionerClient: client,
	}
	bc.AddBucketListener(&bl)

	return bc, nil
}

func (bl *bucketListener) InitializeKubeClient(k kubeclientset.Interface) {
	bl.kubeClient = k

	serverVersion, err := k.Discovery().ServerVersion()
	if err != nil {
		klog.Errorf("unable to get server version: %v", err)
	} else {
		bl.kubeVersion = utilversion.MustParseSemantic(serverVersion.GitVersion)
	}
}

func (bl *bucketListener) InitializeBucketClient(bc bucketclientset.Interface) {
	bl.bucketClient = bc
}

func (bl *bucketListener) Add(ctx context.Context, obj *v1alpha1.Bucket) error {
	klog.V(1).Infof("bucketListener: add called for bucket %s", obj.Name)

	// Verify this bucket is for this provisioner
	if !strings.EqualFold(obj.Spec.Provisioner, bl.provisionerName) {
		return nil
	}

	req := osspec.ProvisionerCreateBucketRequest{
		BucketName: obj.Spec.BucketRequest.Name,
	}

	switch obj.Spec.Protocol.Name {
	case v1alpha1.ProtocolNameS3:
		req.Region = obj.Spec.Protocol.S3.Region
	case v1alpha1.ProtocolNameAzure:
	case v1alpha1.ProtocolNameGCS:
	default:
		klog.Errorf("unknown procotol: %s", obj.Spec.Protocol.Name)
		return nil
	}

	// TODO set grpc timeout
	rsp, err := bl.provisionerClient.ProvisionerCreateBucket(ctx, &req)
	if err != nil {
		klog.Errorf("error calling ProvisionerCreateBucket: %v", err)
		return err
	}
	klog.Infof("provisioner returned create bucket response %v", rsp)

	return nil
}

func (bl *bucketListener) Update(ctx context.Context, old, new *v1alpha1.Bucket) error {
	klog.V(1).Infof("bucketListener: update called for bucket %s", old.Name)
	return nil
}

func (bl *bucketListener) Delete(ctx context.Context, obj *v1alpha1.Bucket) error {
	klog.V(1).Infof("bucketListener: delete called for bucket %s", obj.Name)

	// Verify this bucket is for this provisioner
	if !strings.EqualFold(obj.Spec.Provisioner, bl.provisionerName) {
		return nil
	}

	req := osspec.ProvisionerDeleteBucketRequest{
		BucketName: obj.Spec.BucketRequest.Name,
	}

	switch obj.Spec.Protocol.Name {
	case v1alpha1.ProtocolNameS3:
		req.Region = obj.Spec.Protocol.S3.Region
	case v1alpha1.ProtocolNameAzure:
	case v1alpha1.ProtocolNameGCS:
	default:
		klog.Errorf("unknown procotol: %s", obj.Spec.Protocol.Name)
		return nil
	}

	// TODO set grpc timeout
	rsp, err := bl.provisionerClient.ProvisionerDeleteBucket(ctx, &req)
	if err != nil {
		klog.Errorf("error calling ProvisionerDeleteBucket: %v", err)
		return err
	}
	klog.Infof("provisioner returned delete bucket response %v", rsp)

	return nil
}
