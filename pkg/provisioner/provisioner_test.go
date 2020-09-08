package provisioner

import (
	"context"
	"errors"
	"github.com/container-object-storage-interface/api/apis/cosi.sigs.k8s.io/v1alpha1"
	cosiclient "github.com/container-object-storage-interface/api/clientset/fake"
	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller"
	cosi "github.com/container-object-storage-interface/spec"
	"github.com/container-object-storage-interface/spec/fake"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

// NewBucketProvisioner creates new provisioner for testing
func NewBucketProvisionerTest(t *testing.T, client *fake.MockProvisionerClient) *cosiProvisioner {
	driverName := "testDriver"
	identity := "testProvisioner"

	connectionTimeout := 10 * time.Second

	return &cosiProvisioner{
		client:        kubernetes.NewSimpleClientset(),
		cosiInterface: cosiclient.NewSimpleClientset(),
		grpcClient:    nil,
		cosiClient:    client,
		timeout:       connectionTimeout,
		identity:      identity,
		driverName:    driverName,
	}
}

func TestCreateBucket(t *testing.T) {
	client := &fake.MockProvisionerClient{}
	client.CreateBucket = func(ctx context.Context, in *cosi.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*cosi.ProvisionerCreateBucketResponse, error) {
		return &cosi.ProvisionerCreateBucketResponse{}, nil
	}

	prov := NewBucketProvisionerTest(t, client)
	ctx := context.Background()
	options := controller.BucketProvisionOptions{
		Bucket: &v1alpha1.Bucket{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec:       v1alpha1.BucketSpec{},
			Status:     v1alpha1.BucketStatus{},
		},
	}
	bucket, status, err := prov.CreateBucket(ctx, options)
	if err != nil {
		t.Errorf("TestCreateBucket - failed, expected %v got %v", nil, err)
	} else {
		t.Logf("TestCreateBucket - success, got %v %v %v", bucket, status, err)
	}
}

func TestCreateBucketFail(t *testing.T) {
	client := &fake.MockProvisionerClient{}
	client.CreateBucket = func(ctx context.Context, in *cosi.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*cosi.ProvisionerCreateBucketResponse, error) {
		return &cosi.ProvisionerCreateBucketResponse{}, errors.New("test failed bucket create")
	}

	prov := NewBucketProvisionerTest(t, client)
	ctx := context.Background()
	options := controller.BucketProvisionOptions{}

	bucket, state, err := prov.CreateBucket(ctx, options)
	if err == nil {
		t.Errorf("TestCreateBucketFail - failed, expected error got %v %v %v", bucket, state, err)
	} else {
		t.Logf("TestCreateBucketFail - success %v", err)
	}
}
