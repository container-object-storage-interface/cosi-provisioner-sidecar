module github.com/container-object-storage-interface/cosi-provisioner-sidecar/cosi-sidecar

go 1.14

require (
	github.com/container-object-storage-interface/api v0.0.0-20200924013804-9290d9c83ae2
	github.com/container-object-storage-interface/cosi-provisioner-sidecar v0.0.0-00010101000000-000000000000
	github.com/container-object-storage-interface/spec v0.0.0-20200908192509-18912d8bf2a5
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	golang.org/x/net v0.0.0-20200904194848-62affa334b73
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/grpc v1.31.1
	k8s.io/api v0.18.4
	k8s.io/apimachinery v0.18.4
	k8s.io/client-go v0.18.4
	k8s.io/klog v1.0.0
)

replace github.com/container-object-storage-interface/cosi-provisioner-sidecar => ./
replace github.com/container-object-storage-interface/api => ../api
