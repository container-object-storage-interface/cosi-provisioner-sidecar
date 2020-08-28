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
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	cosiclient "github.com/container-object-storage-interface/api/clientset"

	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/controller"
	ctrl "github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/provisioner"

	cosispec "github.com/container-object-storage-interface/spec"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

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

	provisionController *controller.ProvisionController
	operationTimeout    = flag.Duration("timeout", 10*time.Second, "Timeout for waiting for creation or deletion of a volume")
	retryIntervalStart  = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed provisioning or deletion. It doubles with each failure, up to retry-interval-max.")
	retryIntervalMax    = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed provisioning or deletion.")

	kubeconfig = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Either this or master needs to be set if the provisioner is being run out of cluster.")
	master     = flag.String("master", "", "Master URL to build a client config from. Either this or kubeconfig needs to be set if the provisioner is being run out of cluster.")
)

type options struct {
	reconnect func() bool
}

// Option is the type of all optional parameters for Connect.
type Option func(o *options)

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

	//default should be overridden when the call to driver is made
	provisionerName := "testProvisioner"

	klog.V(1).Infof("Attempting to open a gRPC connection with: %q", *cosiAddress)
	cosiConn, err := connect(*cosiAddress, []grpc.DialOption{}, nil)
	if err != nil {
		klog.Errorf("error connecting to COSI driver: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cosiTimeout)
	defer cancel()

	klog.V(1).Infof("Creating COSI client")
	client := cosispec.NewProvisionerClient(cosiConn)

	klog.Infof("Calling COSI driver to discover driver name")
	req := cosispec.ProvisionerGetInfoRequest{}
	rsp, err := client.ProvisionerGetInfo(ctx, &req)
	if err != nil {
		klog.Errorf("error calling COSI cosispec.ProvisionerGetInfoRequest: %v", err)
		os.Exit(1)
	}
	provisionerName = rsp.ProvisionerIdentity
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
	//fmt.Println("config ", config)
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create client: %v", err)
	}

	cosiclient, err := cosiclient.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create client: %v", err)
	}

	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		klog.Fatalf("Error getting server version: %v", err)
	}

	// Generate a unique ID for this provisioner
	timeStamp := time.Now().UnixNano() / int64(time.Millisecond)
	identity := strconv.FormatInt(timeStamp, 10) + "-" + strconv.Itoa(rand.Intn(10000)) + "-" + provisionerName

	// Create the bucketprovisioner: it implements the BucketProvisioner interface expected by
	// the controller
	cosiBucketProvisioner := ctrl.NewBucketProvisioner(
		clientset,
		cosiclient,
		*operationTimeout,
		identity,
		cosiConn,
		provisionerName,
	)

	// Create the bucketaccessprovisioner: it implements the BucketAccessProvisioner interface expected by
	// the controller
	cosiBucketAccessProvisioner := ctrl.NewBucketAccessProvisioner(
		clientset,
		cosiclient,
		*operationTimeout,
		identity,
		cosiConn,
		provisionerName,
	)

	provisionerOptions := []func(*controller.ProvisionController) error{
		controller.FailedProvisionThreshold(0),
		controller.FailedDeleteThreshold(0),
		controller.RateLimiter(workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax)),
		//                controller.CreateProvisionedPVLimiter(workqueue.DefaultControllerRateLimiter()),
	}

	provisionController = controller.NewProvisionController(
		clientset,
		cosiclient,
		provisionerName,
		cosiBucketProvisioner,
		cosiBucketAccessProvisioner,
		serverVersion.GitVersion,
		provisionerOptions...,
	)

	factory := informers.NewSharedInformerFactory(clientset, ctrl.ResyncPeriodOfCOSINodeInformer)

	run := func(context.Context) {
		stopCh := context.Background().Done()
		factory.Start(stopCh)
		cacheSyncResult := factory.WaitForCacheSync(stopCh)
		for _, v := range cacheSyncResult {
			if !v {
				klog.Fatalf("Failed to sync Informers!")
			}
		}

		provisionController.Run(wait.NeverStop)
	}
	run(context.TODO())
	fmt.Println("Sleep now")
	time.Sleep(1000 * time.Millisecond)
}

// connect is the internal implementation of Connect. It has more options to enable testing.
func connect(
	address string,
	dialOptions []grpc.DialOption, connectOptions []Option) (*grpc.ClientConn, error) {
	var o options
	for _, option := range connectOptions {
		option(&o)
	}

	dialOptions = append(dialOptions,
		grpc.WithInsecure(),                   // Don't use TLS, it's usually local Unix domain socket in a container.
		grpc.WithBackoffMaxDelay(time.Second), // Retry every second after failure.
		grpc.WithBlock(),                      // Block until connection succeeds.
		grpc.WithChainUnaryInterceptor(
			LogGRPC, // Log all messages.
		),
	)
	unixPrefix := "unix://"
	if strings.HasPrefix(address, "tcp://") {
		address = address[6:]
	}
	if strings.HasPrefix(address, "/") {
		// It looks like filesystem path.
		address = unixPrefix + address
	}

	if strings.HasPrefix(address, unixPrefix) {
		// state variables for the custom dialer
		haveConnected := false
		lostConnection := false
		reconnect := true

		dialOptions = append(dialOptions, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			if haveConnected && !lostConnection {
				// We have detected a loss of connection for the first time. Decide what to do...
				// Record this once. TODO (?): log at regular time intervals.
				klog.Errorf("Lost connection to %s.", address)
				// Inform caller and let it decide? Default is to reconnect.
				if o.reconnect != nil {
					reconnect = o.reconnect()
				}
				lostConnection = true
			}
			if !reconnect {
				return nil, errors.New("connection lost, reconnecting disabled")
			}
			conn, err := net.DialTimeout("unix", address[len(unixPrefix):], timeout)
			if err == nil {
				// Connection restablished.
				haveConnected = true
				lostConnection = false
			}
			return conn, err
		}))
	} else if o.reconnect != nil {
		return nil, errors.New("OnConnectionLoss callback only supported for unix:// addresses")
	}

	klog.Infof("Connecting to %s", address)

	// Connect in background.
	var conn *grpc.ClientConn
	var err error
	ready := make(chan bool)
	go func() {
		conn, err = grpc.Dial(address, dialOptions...)
		close(ready)
	}()

	// Log error every connectionLoggingInterval
	ticker := time.NewTicker(connectionLoggingInterval)
	defer ticker.Stop()

	// Wait until Dial() succeeds.
	for {
		select {
		case <-ticker.C:
			klog.Warningf("Still connecting to %s", address)

		case <-ready:
			return conn, err
		}
	}
}
