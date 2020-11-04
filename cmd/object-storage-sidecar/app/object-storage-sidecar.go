package app

import (
	"context"
	"os"
	"time"

	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller/bucket"
	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller/bucketaccess"
	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/grpcclient"

	osspec "github.com/container-object-storage-interface/spec"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

const (
	// Interval of logging connection errors
	connectionLoggingInterval = 10 * time.Second
	defaultDriverAddress      = "tcp://0.0.0.0:9000"
)

type SidecarOptions struct {
	driverAddress string
}

// NewSidecarOptions returns an initialized SidecarOptions instance
func NewSidecarOptions() *SidecarOptions {
	return &SidecarOptions{driverAddress: defaultDriverAddress}
}

func (so *SidecarOptions) Run() {
	var provisionerName string

	klog.V(1).Infof("attempting to open a gRPC connection with: %q", so.driverAddress)
	grpcClient, err := grpcclient.NewGRPCClient(so.driverAddress, []grpc.DialOption{}, nil)
	if err != nil {
		klog.Errorf("error creating GRPC Client: %v", err)
		os.Exit(1)
	}

	grpcConn, err := grpcClient.ConnectWithLogging(connectionLoggingInterval)
	if err != nil {
		klog.Errorf("error connecting to COSI driver: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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

	so.startControllers(ctx, provisionerName, provisionerClient)
	<-ctx.Done()
}

func (so *SidecarOptions) startControllers(ctx context.Context, name string, client osspec.ProvisionerClient) {
	bucketController, err := bucket.NewBucketController(name, client)
	if err != nil {
		klog.Fatalf("Error creating bucket controller: %v", err)
	}

	bucketAccessController, err := bucketaccess.NewBucketAccessController(name, client)
	if err != nil {
		klog.Fatalf("Error creating bucket access controller: %v", err)
	}

	go bucketController.Run(ctx)
	go bucketAccessController.Run(ctx)
}

// NewControllerManagerCommand creates a *cobra.Command object with default parameters
func NewControllerManagerCommand() *cobra.Command {
	opts := NewSidecarOptions()

	cmd := &cobra.Command{
		Use:                   "object-storage-sidecar",
		DisableFlagsInUseLine: true,
		Short:                 "",
		Long:                  ``,
		Run: func(cmd *cobra.Command, args []string) {
			opts.Run()
		},
	}

	cmd.Flags().StringVarP(&opts.driverAddress, "connect-address", "c", opts.driverAddress, "The address that the sidecar should connect to")

	return cmd
}
