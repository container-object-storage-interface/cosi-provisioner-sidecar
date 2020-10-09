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
	"os/signal"
	"syscall"

	"github.com/container-object-storage-interface/cosi-provisioner-sidecar/examples/sample-provisioner/driver"
	server "github.com/container-object-storage-interface/cosi-provisioner-sidecar/pkg/grpcserver"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

var (
	cosiAddress = "tcp://0.0.0.0:9000"
	ctx         context.Context
)

var cmd = &cobra.Command{
	Use:           os.Args[0],
	Short:         "sample provisoner for provisioning bucket instance to the backend bucket",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(c *cobra.Command, args []string) error {
		return run(args, cosiAddress)
	},
	DisableFlagsInUseLine: true,
	Version: driver.VERSION,
}

func init() {
	viper.AutomaticEnv()

	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	flag.Set("logtostderr", "true")

	strFlag := func(c *cobra.Command, ptr *string, name string, short string, dfault string, desc string) {
		c.PersistentFlags().
			StringVarP(ptr, name, short, dfault, desc)
	}
	strFlag(cmd, &cosiAddress, "cosi-address", "", cosiAddress, "Path of the COSI driver socket that the provisioner will connect to.")

	hideFlag := func(name string) {
		cmd.PersistentFlags().MarkHidden(name)
	}
	hideFlag("alsologtostderr")
	hideFlag("log_backtrace_at")
	hideFlag("log_dir")
	hideFlag("logtostderr")
	hideFlag("master")
	hideFlag("stderrthreshold")
	hideFlag("vmodule")

	// suppress the incorrect prefix in glog output
	flag.CommandLine.Parse([]string{})
	viper.BindPFlags(cmd.PersistentFlags())

	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGSEGV)

	go func() {
		s := <-sigs
		cancel()
		panic(fmt.Sprintf("%s %s", s.String(), "Signal received. Exiting"))
	}()

}

func main() {
	if err := cmd.Execute(); err != nil {
		klog.Fatal(err.Error())

	}
}

func run(args []string, endpoint string) error {
	cds := driver.DriverServer{Name: driver.PROVISIONER_NAME, Version: driver.VERSION}
	s := server.NewNonBlockingGRPCServer()
	s.Start(endpoint, &cds)
	s.Wait()
	return nil
}
