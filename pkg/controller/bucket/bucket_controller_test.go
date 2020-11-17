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
	"reflect"
	"testing"

	"github.com/container-object-storage-interface/api/apis/objectstorage.k8s.io/v1alpha1"

	fakebucketclientset "github.com/container-object-storage-interface/api/clientset/fake"

	osspec "github.com/container-object-storage-interface/spec"
	fakespec "github.com/container-object-storage-interface/spec/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/version"

	fakediscovery "k8s.io/client-go/discovery/fake"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"

	"google.golang.org/grpc"
)

func TestInitializeKubeClient(t *testing.T) {
	client := fakekubeclientset.NewSimpleClientset()
	fakeDiscovery, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
	if !ok {
		t.Fatalf("couldn't convert Discovery() to *FakeDiscovery")
	}

	fakeVersion := &version.Info{
		GitVersion: "v1.0.0",
	}
	fakeDiscovery.FakedServerVersion = fakeVersion

	bl := bucketListener{}
	bl.InitializeKubeClient(client)

	if bl.kubeClient == nil {
		t.Errorf("kubeClient was nil")
	}

	expected := utilversion.MustParseSemantic(fakeVersion.GitVersion)
	if !reflect.DeepEqual(expected, bl.kubeVersion) {
		t.Errorf("expected %+v, but got %+v", expected, bl.kubeVersion)
	}
}

func TestInitializeBucketClient(t *testing.T) {
	client := fakebucketclientset.NewSimpleClientset()

	bl := bucketListener{}
	bl.InitializeBucketClient(client)

	if bl.bucketClient == nil {
		t.Errorf("bucketClient was nil")
	}
}

func TestAddWrongProvisioner(t *testing.T) {
	provisioner := "provisioner1"
	mpc := struct{ fakespec.MockProvisionerClient }{}
	mpc.CreateBucket = func(ctx context.Context, in *osspec.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerCreateBucketResponse, error) {
		t.Errorf("grpc client called")
		return nil, nil
	}

	bl := bucketListener{
		provisionerName:   provisioner,
		provisionerClient: &mpc,
	}

	b := v1alpha1.Bucket{
		Spec: v1alpha1.BucketSpec{
			Provisioner: "provisioner2",
		},
	}
	ctx := context.TODO()
	err := bl.Add(ctx, &b)
	if err != nil {
		t.Errorf("error returned: %+v", err)
	}
}

func TestAddValidProtocols(t *testing.T) {
	provisioner := "provisioner1"
	region := "region1"
	bucketName := "bucket1"
	protocolVersion := "proto1"
	sigVersion := v1alpha1.S3SignatureVersion(v1alpha1.S3SignatureVersionV2)
	account := "account1"
	keyName := "keyName1"
	projID := "id1"
	anonAccess := "BUCKET_PRIVATE"
	mpc := struct{ fakespec.MockProvisionerClient }{}

	testCases := []struct {
		name         string
		setProtocol  func(b *v1alpha1.Bucket)
		protocolName v1alpha1.ProtocolName
		createFunc   func(ctx context.Context, in *osspec.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerCreateBucketResponse, error)
		params       map[string]string
	}{
		{
			name: "S3",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.S3 = &v1alpha1.S3Protocol{
					Region:           region,
					Version:          protocolVersion,
					SignatureVersion: sigVersion,
				}
			},
			protocolName: v1alpha1.ProtocolNameS3,
			createFunc: func(ctx context.Context, in *osspec.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerCreateBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Region != region {
					t.Errorf("expected %s, got %s", region, in.Region)
				}
				if in.BucketContext["Version"] != protocolVersion {
					t.Errorf("expected %s, got %s", protocolVersion, in.BucketContext["Version"])
				}
				if in.BucketContext["SignatureVersion"] != string(sigVersion) {
					t.Errorf("expected %s, got %s", sigVersion, in.BucketContext["SignatureVersion"])
				}
				return &osspec.ProvisionerCreateBucketResponse{}, nil
			},
			params: map[string]string{"BucketInstanceName": bucketName},
		},
		{
			name: "GCS",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.GCS = &v1alpha1.GCSProtocol{
					ServiceAccount: account,
					PrivateKeyName: keyName,
					ProjectID:      projID,
				}
			},
			protocolName: v1alpha1.ProtocolNameGCS,
			createFunc: func(ctx context.Context, in *osspec.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerCreateBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.BucketContext["ServiceAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ServiceAccount"])
				}
				if in.BucketContext["PrivateKeyName"] != keyName {
					t.Errorf("expected %s, got %s", region, in.BucketContext["PrivateKeyName"])
				}
				if in.BucketContext["ProjectID"] != projID {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ProjectID"])
				}
				return &osspec.ProvisionerCreateBucketResponse{}, nil
			},
			params: map[string]string{"BucketInstanceName": bucketName},
		},
		{
			name: "AzureBlob",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.AzureBlob = &v1alpha1.AzureProtocol{
					StorageAccount: account,
				}
			},
			protocolName: v1alpha1.ProtocolNameAzure,
			createFunc: func(ctx context.Context, in *osspec.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerCreateBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.BucketContext["StorageAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["StorageAccount"])
				}
				return &osspec.ProvisionerCreateBucketResponse{}, nil
			},
			params: map[string]string{"BucketInstanceName": bucketName},
		},
		{
			name: "AnonymousAccessMode",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.AzureBlob = &v1alpha1.AzureProtocol{
					StorageAccount: account,
				}
			},
			protocolName: v1alpha1.ProtocolNameAzure,
			createFunc: func(ctx context.Context, in *osspec.ProvisionerCreateBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerCreateBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.BucketContext["StorageAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["StorageAccount"])
				}
				aMode := osspec.ProvisionerCreateBucketRequest_AnonymousBucketAccessMode(osspec.ProvisionerCreateBucketRequest_AnonymousBucketAccessMode_value[anonAccess])
				if in.AnonymousBucketAccessMode != aMode {
					t.Errorf("expected %s, got %s", aMode, in.AnonymousBucketAccessMode)
				}
				return &osspec.ProvisionerCreateBucketResponse{}, nil
			},
			params: map[string]string{
				"AnonymousAccessMode": anonAccess,
			},
		},
	}

	for _, tc := range testCases {
		b := v1alpha1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name: bucketName,
			},
			Spec: v1alpha1.BucketSpec{
				Provisioner: provisioner,
				Protocol: v1alpha1.Protocol{
					RequestedProtocol: v1alpha1.RequestedProtocol{
						Name: tc.protocolName,
					},
				},
				Parameters: tc.params,
			},
		}

		ctx := context.TODO()
		client := fakebucketclientset.NewSimpleClientset(&b)
		kubeClient := fakekubeclientset.NewSimpleClientset()
		mpc.CreateBucket = tc.createFunc
		bl := bucketListener{
			provisionerName:   provisioner,
			provisionerClient: &mpc,
			bucketClient:      client,
			kubeClient:        kubeClient,
		}

		tc.setProtocol(&b)
		t.Logf("Testing protocol %s", tc.name)
		err := bl.Add(ctx, &b)
		if err != nil {
			t.Errorf("add returned: %+v", err)
		}

		updatedB, _ := client.ObjectstorageV1alpha1().Buckets().Get(ctx, b.Name, metav1.GetOptions{})
		if updatedB.Status.BucketAvailable != true {
			t.Errorf("expected %t, got %t", true, b.Status.BucketAvailable)
		}
	}
}

func TestAddInvalidProtocol(t *testing.T) {
	const (
		protocolName v1alpha1.ProtocolName = "invalid"
	)

	bucketName := "bucket1"
	provisioner := "provisioner1"

	bl := bucketListener{
		provisionerName: provisioner,
	}

	b := v1alpha1.Bucket{
		Spec: v1alpha1.BucketSpec{
			BucketRequest: &v1alpha1.ObjectReference{
				Name: bucketName,
			},
			Provisioner: provisioner,
			Protocol: v1alpha1.Protocol{
				RequestedProtocol: v1alpha1.RequestedProtocol{
					Name: protocolName,
				},
			},
		},
	}

	ctx := context.TODO()
	err := bl.Add(ctx, &b)
	if err == nil {
		t.Errorf("invalidProtocol: no error returned")
	}
}

func TestDeleteWrongProvisioner(t *testing.T) {
	provisioner := "provisioner1"
	mpc := struct{ fakespec.MockProvisionerClient }{}
	mpc.DeleteBucket = func(ctx context.Context, in *osspec.ProvisionerDeleteBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerDeleteBucketResponse, error) {
		t.Errorf("grpc client called")
		return nil, nil
	}

	bl := bucketListener{
		provisionerName:   provisioner,
		provisionerClient: &mpc,
	}

	b := v1alpha1.Bucket{
		Spec: v1alpha1.BucketSpec{
			Provisioner: "provisioner2",
		},
	}
	ctx := context.TODO()
	err := bl.Delete(ctx, &b)
	if err != nil {
		t.Errorf("error returned: %+v", err)
	}
}

func TestDeleteValidProtocols(t *testing.T) {
	provisioner := "provisioner1"
	region := "region1"
	bucketName := "bucket1"
	protocolVersion := "proto1"
	sigVersion := v1alpha1.S3SignatureVersion(v1alpha1.S3SignatureVersionV2)
	account := "account1"
	keyName := "keyName1"
	projID := "id1"
	endpoint := "endpoint1"
	mpc := struct{ fakespec.MockProvisionerClient }{}

	testCases := []struct {
		name         string
		setProtocol  func(b *v1alpha1.Bucket)
		protocolName v1alpha1.ProtocolName
		deleteFunc   func(ctx context.Context, in *osspec.ProvisionerDeleteBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerDeleteBucketResponse, error)
	}{
		{
			name: "S3",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.S3 = &v1alpha1.S3Protocol{
					Region:           region,
					Version:          protocolVersion,
					SignatureVersion: sigVersion,
					BucketName:       bucketName,
					Endpoint:         endpoint,
				}
			},
			protocolName: v1alpha1.ProtocolNameS3,
			deleteFunc: func(ctx context.Context, in *osspec.ProvisionerDeleteBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerDeleteBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Region != region {
					t.Errorf("expected %s, got %s", region, in.Region)
				}
				if in.BucketContext["Version"] != protocolVersion {
					t.Errorf("expected %s, got %s", protocolVersion, in.BucketContext["Version"])
				}
				if in.BucketContext["SignatureVersion"] != string(sigVersion) {
					t.Errorf("expected %s, got %s", sigVersion, in.BucketContext["SignatureVersion"])
				}
				if in.BucketContext["Endpoint"] != endpoint {
					t.Errorf("expected %s, got %s", endpoint, in.BucketContext["Endpoint"])
				}
				return &osspec.ProvisionerDeleteBucketResponse{}, nil
			},
		},
		{
			name: "GCS",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.GCS = &v1alpha1.GCSProtocol{
					ServiceAccount: account,
					PrivateKeyName: keyName,
					ProjectID:      projID,
					BucketName:     bucketName,
				}
			},
			protocolName: v1alpha1.ProtocolNameGCS,
			deleteFunc: func(ctx context.Context, in *osspec.ProvisionerDeleteBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerDeleteBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.BucketContext["ServiceAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ServiceAccount"])
				}
				if in.BucketContext["PrivateKeyName"] != keyName {
					t.Errorf("expected %s, got %s", region, in.BucketContext["PrivateKeyName"])
				}
				if in.BucketContext["ProjectID"] != projID {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ProjectID"])
				}
				return &osspec.ProvisionerDeleteBucketResponse{}, nil
			},
		},
		{
			name: "AzureBlob",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.AzureBlob = &v1alpha1.AzureProtocol{
					StorageAccount: account,
					ContainerName:  bucketName,
				}
			},
			protocolName: v1alpha1.ProtocolNameAzure,
			deleteFunc: func(ctx context.Context, in *osspec.ProvisionerDeleteBucketRequest, opts ...grpc.CallOption) (*osspec.ProvisionerDeleteBucketResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.BucketContext["StorageAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["StorageAccount"])
				}
				return &osspec.ProvisionerDeleteBucketResponse{}, nil
			},
		},
	}

	for _, tc := range testCases {
		b := v1alpha1.Bucket{
			Spec: v1alpha1.BucketSpec{
				Provisioner: provisioner,
				Protocol: v1alpha1.Protocol{
					RequestedProtocol: v1alpha1.RequestedProtocol{
						Name: tc.protocolName,
					},
				},
			},
			Status: v1alpha1.BucketStatus{
				BucketAvailable: true,
			},
		}

		ctx := context.TODO()
		client := fakebucketclientset.NewSimpleClientset(&b)
		mpc.DeleteBucket = tc.deleteFunc
		bl := bucketListener{
			provisionerName:   provisioner,
			provisionerClient: &mpc,
			bucketClient:      client,
		}

		tc.setProtocol(&b)
		t.Logf("Testing protocol %s", tc.name)
		err := bl.Delete(ctx, &b)
		if err != nil {
			t.Errorf("delete returned: %+v", err)
		}

		updatedB, _ := client.ObjectstorageV1alpha1().Buckets().Get(ctx, b.Name, metav1.GetOptions{})
		if updatedB.Status.BucketAvailable != false {
			t.Errorf("expected %t, got %t", false, b.Status.BucketAvailable)
		}
	}
}

func TestDeleteInvalidProtocol(t *testing.T) {
	const (
		protocolName v1alpha1.ProtocolName = "invalid"
	)

	bucketName := "bucket1"
	provisioner := "provisioner1"

	bl := bucketListener{
		provisionerName: provisioner,
	}

	b := v1alpha1.Bucket{
		Spec: v1alpha1.BucketSpec{
			BucketRequest: &v1alpha1.ObjectReference{
				Name: bucketName,
			},
			Provisioner: provisioner,
			Protocol: v1alpha1.Protocol{
				RequestedProtocol: v1alpha1.RequestedProtocol{
					Name: protocolName,
				},
			},
		},
	}

	ctx := context.TODO()
	err := bl.Delete(ctx, &b)
	if err == nil {
		t.Errorf("invalidProtocol: no error returned")
	}
}
