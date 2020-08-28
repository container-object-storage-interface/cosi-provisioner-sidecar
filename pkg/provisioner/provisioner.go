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

package provisioner

import (
	"context"
	"time"

	"github.com/container-object-storage-interface/api/apis/cosi.sigs.k8s.io/v1alpha1"
	cosiclient "github.com/container-object-storage-interface/api/clientset"

	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller"

	"github.com/container-object-storage-interface/spec/lib/go/cosi"

	_ "k8s.io/apimachinery/pkg/util/json"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"k8s.io/klog"

	"google.golang.org/grpc"
)

const (
	// COSI Parameters prefixed with cosiParameterPrefix are not passed through
	// to the driver on CreateBucket calls. Instead they are intended
	// to used by the COSI external-provisioner and maybe used to populate
	// fields in subsequent COSI calls or Kubernetes API objects.
	cosiParameterPrefix = "cosi.storage.k8s.io/"

	// Bucket and BucketRequest metadata, used for sending to drivers in the  create requests, added as parameters, optional.
	bucketRequestNameKey      = "csi.storage.k8s.io/pvc/name"
	bucketRequestNamespaceKey = "csi.storage.k8s.io/pvc/namespace"
	bucketNameKey             = "csi.storage.k8s.io/pv/name"

	// Defines parameters for ExponentialBackoff used for executing
	// CSI CreateVolume API call, it gives approx 4 minutes for the CSI
	// driver to complete a volume creation.
	backoffDuration = time.Second * 5
	backoffFactor   = 1.2
	backoffSteps    = 10

	tokenBucketNameKey             = "pv.name"
	tokenBucketRequestNameKey      = "pvc.name"
	tokenBucketRequestNameSpaceKey = "pvc.namespace"

	ResyncPeriodOfCOSINodeInformer = 1 * time.Hour

	deleteVolumeRetryCount = 5

	annStorageProvisioner = "bucket.beta.kubernetes.io/cosi-provisioner"
)

// COSIProvisioner struct
type cosiProvisioner struct {
	client        kubernetes.Interface
	cosiInterface cosiclient.Interface
	cosiClient    cosi.CosiControllerClient
	grpcClient    *grpc.ClientConn
	timeout       time.Duration
	identity      string
	config        *rest.Config
	driverName    string
}

var _ controller.BucketProvisioner = &cosiProvisioner{}
var _ controller.BucketAccessProvisioner = &cosiProvisioner{}

var (
	// Each provisioner have a identify string to distinguish with others. This
	// identify string will be added in Bucket annotations under this key.
	provisionerIDKey = "storage.kubernetes.io/cosiProvisionerIdentity"
)

// NewBucketProvisioner creates new  provisioner
func NewBucketProvisioner(client kubernetes.Interface,
	client_cosi cosiclient.Interface,
	connectionTimeout time.Duration,
	identity string,
	grpcClient *grpc.ClientConn,
	driverName string,
) controller.BucketProvisioner {

	cosiClient := cosi.NewCosiControllerClient(grpcClient)

	provisioner := &cosiProvisioner{
		client:        client,
		cosiInterface: client_cosi,
		grpcClient:    grpcClient,
		cosiClient:    cosiClient,
		timeout:       connectionTimeout,
		identity:      identity,
		driverName:    driverName,
	}
	return provisioner
}

// NewBucketAccessProvisioner creates new  provisioner
func NewBucketAccessProvisioner(client kubernetes.Interface,
	client_cosi cosiclient.Interface,
	connectionTimeout time.Duration,
	identity string,
	grpcClient *grpc.ClientConn,
	driverName string,
) controller.BucketAccessProvisioner {

	cosiClient := cosi.NewCosiControllerClient(grpcClient)

	provisioner := &cosiProvisioner{
		client:        client,
		cosiInterface: client_cosi,
		grpcClient:    grpcClient,
		cosiClient:    cosiClient,
		timeout:       connectionTimeout,
		identity:      identity,
		driverName:    driverName,
	}
	return provisioner
}

func (p *cosiProvisioner) CreateBucket(ctx context.Context, options controller.BucketProvisionOptions) (*v1alpha1.Bucket, controller.ProvisioningState, error) {
	klog.V(1).Infof("Calling COSI driver to create bucket")
	client := cosi.NewCosiControllerClient(p.grpcClient)
	req := cosi.CreateBucketRequest{}
	rsp, err := client.CreateBucket(ctx, &req)
	if err != nil {
		klog.Errorf("error calling COSI cosi.ProvisionerGetInfoRequest: %v", err)
	}
	klog.Info("This provsioner returned createbucket response %v", rsp)

	bucket := options.Bucket

	// TODO process response, bucket.Name,  bucket.Available and other info
	return bucket, controller.ProvisioningFinished, nil
}

func (p *cosiProvisioner) CreateBucketAccess(context.Context, controller.BucketAccessProvisionOptions) (*v1alpha1.BucketAccess, controller.ProvisioningState, error) {
	return nil, "", nil
}
