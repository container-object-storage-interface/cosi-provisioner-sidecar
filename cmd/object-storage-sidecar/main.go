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
	"time"

	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller/bucket"
	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller/bucketaccess"
	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/grpcclient"

	osspec "github.com/container-object-storage-interface/spec"

	"google.golang.org/grpc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/klog"
)

const (
	// Default timeout of short COSI calls like CreateBucket, etc
	cosiTimeout = time.Second

	// Verify (and update, if needed) the node ID at this freqeuency.
	sleepDuration = 2 * time.Minute

	// Interval of logging connection errors
	connectionLoggingInterval = 10 * time.Second
)

// Command line flags
var (
	connectionTimeout = flag.Duration("connection-timeout", 0, "The --connection-timeout flag is deprecated")
	cosiAddress       = flag.String("cosi-address", "tcp://0.0.0.0:9000", "Path of the COSI driver socket that the provisioner  will connect to.")
	showVersion       = flag.Bool("version", false, "Show version.")
	version           = "unknown"
	config            *rest.Config
	// List of supported versions
	supportedVersions = []string{"1.0.0"}

	//	provisionController *controller.ProvisionController
	operationTimeout   = flag.Duration("timeout", 10*time.Second, "Timeout for waiting for creation or deletion of a volume")
	retryIntervalStart = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed provisioning or deletion. It doubles with each failure, up to retry-interval-max.")
	retryIntervalMax   = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed provisioning or deletion.")

	kubeconfig = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Either this or master needs to be set if the provisioner is being run out of cluster.")
	master     = flag.String("master", "", "Master URL to build a client config from. Either this or kubeconfig needs to be set if the provisioner is being run out of cluster.")
)

type options struct {
	reconnect func() bool
}

// Option is the type of all optional parameters for Connect.
type Option func(o *options)

func main() {
	flag.CommandLine.Parse([]string{})
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		return
	}
	klog.Infof("Version: %s", version)

	//default should be overridden when the call to driver is made
	provisionerName := "testProvisioner"

	klog.V(1).Infof("attempting to open a gRPC connection with: %q", *cosiAddress)
	grpcClient, err := grpcclient.NewGRPCClient(*cosiAddress, []grpc.DialOption{}, nil)
	if err != nil {
		klog.Errorf("error creating GRPC Client: %v", err)
		os.Exit(1)
	}

	grpcConn, err := grpcClient.ConnectWithLogging(connectionLoggingInterval)
	if err != nil {
		klog.Errorf("error connecting to COSI driver: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cosiTimeout)
	defer cancel()

	klog.V(1).Infof("creating provisioner client")
	provisionerClient := osspec.NewProvisionerClient(grpcConn)

	klog.Infof("discovering driver name")
	req := osspec.ProvisionerGetInfoRequest{}
	rsp, err := provisionerClient.ProvisionerGetInfo(ctx, &req)
	if err != nil {
		klog.Errorf("error calling ProvisionerGetInfo: %v", err)
		os.Exit(1)
	}

	provisionerName = rsp.ProvisionerIdentity
	// TODO: Register provisioner using internal type
	klog.Info("This provsioner is working with the driver identified as: ", provisionerName)

	// get the KUBECONFIG from env if specified (useful for local/debug cluster)
	kubeconfigEnv := os.Getenv("KUBECONFIG")

	if kubeconfigEnv != "" {
		klog.Infof("Found KUBECONFIG environment variable set, using that..")
		kubeconfig = &kubeconfigEnv
	}

	if *master != "" || *kubeconfig != "" {
		klog.Infof("Either master or kubeconfig specified. building kube config from that..")
		config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	} else {
		klog.Infof("Building kube configs for running in cluster...")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		klog.Fatalf("Failed to create config: %v", err)
	}

	bucketController, err := bucket.NewBucketController(provisionerName, provisionerClient)
	if err != nil {
		klog.Fatalf("Error creating bucket controller: %v", err)
	}

	bucketAccessController, err := bucketaccess.NewBucketAccessController(provisionerName, provisionerClient)
	if err != nil {
		klog.Fatalf("Error creating bucket access controller: %v", err)
	}

	go bucketController.Run(ctx)
	go bucketAccessController.Run(ctx)
	<-ctx.Done()
}
