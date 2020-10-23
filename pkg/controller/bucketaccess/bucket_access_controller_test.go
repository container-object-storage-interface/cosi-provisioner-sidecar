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

func TestAddValidProtocols(t *testing.T) {
	provisioner := "provisioner1"
	region := "region1"
	bucketName := "bucket1"
	principal := "principal1"
	mpc := struct{ fakespec.MockProvisionerClient }{}

	testCases := []struct {
		name      string
		grantFunc func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error)
	}{
		{
			name: "S3",
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
				return &osspec.ProvisionerGrantBucketAccessResponse{}, nil
			},
		},
		{
			name: "GCS",
			grantFunc: func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
				}
				return &osspec.ProvisionerGrantBucketAccessResponse{}, nil
			},
		},
		{
			name: "AzureBlob",
			grantFunc: func(ctx context.Context, in *osspec.ProvisionerGrantBucketAccessRequest, opts ...grpc.CallOption) (*osspec.ProvisionerGrantBucketAccessResponse, error) {
				if in.BucketName != bucketName {
					t.Errorf("expected %s, got %s", bucketName, in.BucketName)
				}
				if in.Principal != principal {
					t.Errorf("expected %s, got %s", principal, in.Principal)
				}
				return &osspec.ProvisionerGrantBucketAccessResponse{}, nil
			},
		},
	}

	for _, tc := range testCases {
		ba := v1alpha1.BucketAccess{
			Spec: v1alpha1.BucketAccessSpec{
				BucketInstanceName: bucketName,
				Provisioner:        provisioner,
				Principal:          principal,
			},
		}

		ctx := context.TODO()
		client := fakebucketclientset.NewSimpleClientset(&ba)
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
