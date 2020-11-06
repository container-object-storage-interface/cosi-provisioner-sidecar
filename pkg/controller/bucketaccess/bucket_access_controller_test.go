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

package bucketaccess

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

	bal := bucketAccessListener{}
	bal.InitializeKubeClient(client)

	if bal.kubeClient == nil {
		t.Errorf("kubeClient was nil")
	}

	expected := utilversion.MustParseSemantic(fakeVersion.GitVersion)
	if !reflect.DeepEqual(expected, bal.kubeVersion) {
		t.Errorf("expected %+v, but got %+v", expected, bal.kubeVersion)
	}
}

func TestInitializeBucketClient(t *testing.T) {
	client := fakebucketclientset.NewSimpleClientset()

	bal := bucketAccessListener{}
	bal.InitializeBucketClient(client)

	if bal.bucketAccessClient == nil {
		t.Errorf("bucketClient was nil")
	}
}

func TestAddWrongProvisioner(t *testing.T) {
	provisioner := "provisioner1"
	mpc := struct{ fakespec.MockProvisionerClient }{}
	mpc.GrantBucketAccess = func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error) {
		t.Errorf("grpc client called")
		return nil, nil
	}

	bal := bucketAccessListener{
		provisionerName:   provisioner,
		provisionerClient: &mpc,
	}

	ba := v1alpha1.BucketAccess{
		Spec: v1alpha1.BucketAccessSpec{
			Provisioner: "provisioner2",
		},
	}
	ctx := context.TODO()
	err := bal.Add(ctx, &ba)
	if err != nil {
		t.Errorf("error returned: %+v", err)
	}
}

func TestAdd(t *testing.T) {
	provisioner := "provisioner1"
	region := "region1"
	bucketName := "bucket1"
	principal := "principal1"
	protocolVersion := "proto1"
	sigVersion := v1alpha1.S3SignatureVersion(v1alpha1.S3SignatureVersionV2)
	account := "account1"
	keyName := "keyName1"
	projId := "id1"
	endpoint := "endpoint1"
	instanceName := "instance"
	mpc := struct{ fakespec.MockProvisionerClient }{}

	testCases := []struct {
		name         string
		setProtocol  func(b *v1alpha1.Bucket)
		protocolName v1alpha1.ProtocolName
		grantFunc    func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error)
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
			grantFunc: func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Region != region {
					t.Errorf("expected %s, got %s", region, in.Region)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
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
				return &osspec.ProvisionerGrantBucketAccessResponse{}, nil
			},
		},
		{
			name: "GCS",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.GCS = &v1alpha1.GCSProtocol{
					ServiceAccount: account,
					PrivateKeyName: keyName,
					ProjectID:      projId,
					BucketName:     bucketName,
				}
			},
			protocolName: v1alpha1.ProtocolNameGCS,
			grantFunc: func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
				}
				if in.BucketContext["ServiceAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ServiceAccount"])
				}
				if in.BucketContext["PrivateKeyName"] != keyName {
					t.Errorf("expected %s, got %s", region, in.BucketContext["PrivateKeyName"])
				}
				if in.BucketContext["ProjectID"] != projId {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ProjectID"])
				}
				return &osspec.ProvisionerGrantBucketAccessResponse{}, nil
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
			grantFunc: func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
				}
				if in.BucketContext["StorageAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["StorageAccount"])
				}
				return &osspec.ProvisionerGrantBucketAccessResponse{}, nil
			},
		},
	}

	for _, tc := range testCases {
		b := v1alpha1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name: instanceName,
			},
			Spec: v1alpha1.BucketSpec{
				Provisioner: provisioner,
				Protocol: v1alpha1.Protocol{
					RequestedProtocol: v1alpha1.RequestedProtocol{
						Name: tc.protocolName,
					},
				},
			},
		}

		ba := v1alpha1.BucketAccess{
			Spec: v1alpha1.BucketAccessSpec{
				BucketInstanceName: instanceName,
				Provisioner:        provisioner,
				Principal:          principal,
			},
		}

		ctx := context.TODO()
		tc.setProtocol(&b)
		client := fakebucketclientset.NewSimpleClientset(&ba, &b)
		mpc.GrantBucketAccess = tc.grantFunc
		bal := bucketAccessListener{
			provisionerName:    provisioner,
			provisionerClient:  &mpc,
			bucketAccessClient: client,
		}

		err := bal.Add(ctx, &ba)
		if err != nil {
			t.Errorf("add returned: %+v", err)
		}
		if ba.Status.AccessGranted != true {
			t.Errorf("expected %t, got %t", true, ba.Status.AccessGranted)
		}
	}
}

func TestDeleteWrongProvisioner(t *testing.T) {
	provisioner := "provisioner1"
	mpc := struct{ fakespec.MockProvisionerClient }{}
	mpc.RevokeBucketAccess = func(ctx context.Context, in *osspec.ProvisionerRevokeBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerRevokeBucketAccessResponse, error) {
		t.Errorf("grpc client called")
		return nil, nil
	}

	bal := bucketAccessListener{
		provisionerName:   provisioner,
		provisionerClient: &mpc,
	}

	ba := v1alpha1.BucketAccess{
		Spec: v1alpha1.BucketAccessSpec{
			Provisioner: "provisioner2",
		},
	}
	ctx := context.TODO()
	err := bal.Delete(ctx, &ba)
	if err != nil {
		t.Errorf("error returned: %+v", err)
	}
}

func TestDelete(t *testing.T) {
	provisioner := "provisioner1"
	region := "region1"
	bucketName := "bucket1"
	principal := "principal1"
	protocolVersion := "proto1"
	sigVersion := v1alpha1.S3SignatureVersion(v1alpha1.S3SignatureVersionV2)
	account := "account1"
	keyName := "keyName1"
	projId := "id1"
	endpoint := "endpoint1"
	instanceName := "instance"
	mpc := struct{ fakespec.MockProvisionerClient }{}

	testCases := []struct {
		name         string
		setProtocol  func(b *v1alpha1.Bucket)
		protocolName v1alpha1.ProtocolName
		revokeFunc   func(ctx context.Context, in *osspec.ProvisionerRevokeBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerRevokeBucketAccessResponse, error)
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
			revokeFunc: func(ctx context.Context, in *osspec.ProvisionerRevokeBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerRevokeBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Region != region {
					t.Errorf("expected %s, got %s", region, in.Region)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
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
				return &osspec.ProvisionerRevokeBucketAccessResponse{}, nil
			},
		},
		{
			name: "GCS",
			setProtocol: func(b *v1alpha1.Bucket) {
				b.Spec.Protocol.GCS = &v1alpha1.GCSProtocol{
					ServiceAccount: account,
					PrivateKeyName: keyName,
					ProjectID:      projId,
					BucketName:     bucketName,
				}
			},
			protocolName: v1alpha1.ProtocolNameGCS,
			revokeFunc: func(ctx context.Context, in *osspec.ProvisionerRevokeBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerRevokeBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
				}
				if in.BucketContext["ServiceAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ServiceAccount"])
				}
				if in.BucketContext["PrivateKeyName"] != keyName {
					t.Errorf("expected %s, got %s", region, in.BucketContext["PrivateKeyName"])
				}
				if in.BucketContext["ProjectID"] != projId {
					t.Errorf("expected %s, got %s", region, in.BucketContext["ProjectID"])
				}
				return &osspec.ProvisionerRevokeBucketAccessResponse{}, nil
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
			revokeFunc: func(ctx context.Context, in *osspec.ProvisionerRevokeBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerRevokeBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
				}
				if in.BucketContext["StorageAccount"] != account {
					t.Errorf("expected %s, got %s", region, in.BucketContext["StorageAccount"])
				}
				return &osspec.ProvisionerRevokeBucketAccessResponse{}, nil
			},
		},
	}

	for _, tc := range testCases {
		b := v1alpha1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name: instanceName,
			},
			Spec: v1alpha1.BucketSpec{
				Provisioner: provisioner,
				Protocol: v1alpha1.Protocol{
					RequestedProtocol: v1alpha1.RequestedProtocol{
						Name: tc.protocolName,
					},
				},
			},
		}

		ba := v1alpha1.BucketAccess{
			Spec: v1alpha1.BucketAccessSpec{
				BucketInstanceName: instanceName,
				Provisioner:        provisioner,
				Principal:          principal,
			},
		}

		ctx := context.TODO()
		tc.setProtocol(&b)
		client := fakebucketclientset.NewSimpleClientset(&ba, &b)
		mpc.RevokeBucketAccess = tc.revokeFunc
		bal := bucketAccessListener{
			provisionerName:    provisioner,
			provisionerClient:  &mpc,
			bucketAccessClient: client,
		}

		err := bal.Delete(ctx, &ba)
		if err != nil {
			t.Errorf("delete returned: %+v", err)
		}
		if ba.Status.AccessGranted != false {
			t.Errorf("expected %t, got %t", true, ba.Status.AccessGranted)
		}
	}
}
