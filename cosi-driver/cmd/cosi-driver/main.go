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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"k8s.io/klog"

	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/cosi-driver/pkg/server"
	"github.com/container-object-storage-interface/spec/lib/go/cosi"
)

// Command line flags
var (
	cosiAddress = flag.String("cosi-address", "tcp://0.0.0.0:9000", "Path of the COSI driver socket that the provisioner  will connect to.")
	showVersion = flag.Bool("version", false, "Show version.")
	version     = "unknown"
	// List of supported versions
	supportedVersions = []string{"0.0.1"}
)

// LogGRPC is gPRC unary interceptor for logging of CSI messages at level 5. It removes any secrets from the message.
func LogGRPC(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	klog.V(5).Infof("GRPC call: %s", method)
	klog.V(5).Infof("GRPC request: %s", req)
	err := invoker(ctx, method, req, reply, cc, opts...)
	klog.V(5).Infof("GRPC response: %s", reply)
	klog.V(5).Infof("GRPC error: %v", err)
	return err
}

func main() {
	flag.CommandLine.Parse([]string{})
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		return
	}
	klog.Infof("Version: %s", version)

	cds := server.DriverServer{"testDriver", "1.0"}
	Serve(*cosiAddress, "default", &cds)
}

func Serve(endpoint, ns string, cds cosi.ProvisionerServer) {
	s := server.NewNonBlockingGRPCServer()
	s.Start(endpoint, cds)
	s.Wait()
}
