module github.com/container-object-storage-interface/cosi-provisioner-sidecar/cosi-provisioner

go 1.14

require (
	github.com/container-object-storage-interface/api v0.0.0-20200708183033-b21b31b712bd
	github.com/container-object-storage-interface/spec v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.0.0-20200707034311-ab3426394381 // indirect
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	google.golang.org/genproto v0.0.0-20191220175831-5c49e3ecc1c1 // indirect
	google.golang.org/grpc v1.30.0
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v0.18.4
	k8s.io/klog v1.0.0
)

replace github.com/container-object-storage-interface/api => ../../api

replace github.com/container-object-storage-interface/spec => ../../spec
