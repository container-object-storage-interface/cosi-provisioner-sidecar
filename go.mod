module github.com/container-object-storage-interface/cosi-provisioner-sidecar/cosi-sidecar

go 1.14

require (
	github.com/container-object-storage-interface/api v0.0.0-20200811180640-700a0bdd7111
	github.com/container-object-storage-interface/cosi-provisioner-sidecar v0.0.0-00010101000000-000000000000
	github.com/container-object-storage-interface/cosi-provisioner-sidecar/cosi-driver v0.0.0-20200820223129-65954169b951
	github.com/container-object-storage-interface/spec v0.0.0-20200730055129-568259263ead
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/grpc v1.30.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/api v0.18.4
	k8s.io/apimachinery v0.18.4
	k8s.io/client-go v0.18.4
	k8s.io/klog v1.0.0
)

replace github.com/container-object-storage-interface/cosi-provisioner-sidecar => ./
