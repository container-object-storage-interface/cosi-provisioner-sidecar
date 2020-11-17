module github.com/container-object-storage-interface/cosi-provisioner-sidecar

go 1.13

require (
	github.com/container-object-storage-interface/api v0.0.0-20201102203747-fb97594fb7a4
	github.com/container-object-storage-interface/spec v0.0.0-20200908192509-18912d8bf2a5
	github.com/go-ini/ini v1.62.0 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.8.1
	github.com/minio/minio v0.0.0-20201102154311-4c773f7068fc
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/spf13/cobra v1.0.0
	github.com/spf13/viper v1.7.1
	golang.org/x/net v0.0.0-20201009032441-dbdefad45b89
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	google.golang.org/grpc v1.33.0
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/klog v1.0.0
)
