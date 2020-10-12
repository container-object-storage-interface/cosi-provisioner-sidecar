module github.com/container-object-storage-interface/cosi-provisioner-sidecar

go 1.13

require (
	github.com/container-object-storage-interface/spec v0.0.0-20200908192509-18912d8bf2a5
	github.com/go-ini/ini v1.62.0 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.8.1
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/spf13/cobra v1.0.0
	github.com/spf13/viper v1.7.1
	golang.org/x/net v0.0.0-20201009032441-dbdefad45b89
	google.golang.org/grpc v1.33.0
	k8s.io/klog v1.0.0
)
