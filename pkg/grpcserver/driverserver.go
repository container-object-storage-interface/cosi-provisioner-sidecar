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

package grpcserver

import (
	"fmt"

	"github.com/container-object-storage-interface/spec/lib/go/cosi"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DriverServer struct {
	Name, Version string
}

func (ds *DriverServer) ProvisionerGetInfo(context.Context, *cosi.ProvisionerGetInfoRequest) (*cosi.ProvisionerGetInfoResponse, error) {
	rsp := &cosi.ProvisionerGetInfoResponse{}
	rsp.ProvisionerIdentity = fmt.Sprintf("%s-%s", ds.Name, ds.Version)
	return rsp, nil
}

func (ds DriverServer) ProvisionerCreateBucket(ctx context.Context, req *cosi.ProvisionerCreateBucketRequest) (*cosi.ProvisionerCreateBucketResponse, error) {
	glog.V(5).Infof("Using mocked CreateBucket call")

	if ds.Name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	if ds.Version == "" {
		return nil, status.Error(codes.Unavailable, "Driver is missing version")
	}

	return &cosi.ProvisionerCreateBucketResponse{}, nil
}

func (ds *DriverServer) ProvisionerDeleteBucket(ctx context.Context, req *cosi.ProvisionerDeleteBucketRequest) (*cosi.ProvisionerDeleteBucketResponse, error) {
	return nil, status.Error(codes.Unavailable, "Method not implemented")
}

func (ds *DriverServer) ProvisionerGrantBucketAccess(ctx context.Context, req *cosi.ProvisionerGrantBucketAccessRequest) (*cosi.ProvisionerGrantBucketAccessResponse, error) {
	return nil, status.Error(codes.Unavailable, "Method not implemented")
}

func (ds *DriverServer) ProvisionerRevokeBucketAccess(ctx context.Context, req *cosi.ProvisionerRevokeBucketAccessRequest) (*cosi.ProvisionerRevokeBucketAccessResponse, error) {
	return nil, status.Error(codes.Unavailable, "Method not implemented")
}
