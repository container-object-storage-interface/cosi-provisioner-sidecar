module github.com/container-object-storage-interface/cosi-provisioner-sidecar/cosi-driver

go 1.14

require (
	github.com/container-object-storage-interface/spec v0.0.0-20200622154246-bc84d8cb63a1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	google.golang.org/grpc v1.30.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/klog v1.0.0
)

replace github.com/container-object-storage-interface/spec => ../../spec
