module github.com/openllb/hlb

go 1.12

require (
	github.com/alecthomas/participle v0.4.2-0.20191230055107-1fbf95471489
	github.com/docker/buildx v0.3.1
	github.com/docker/cli v1.14.0-0.20190523191156-ab688a9a79a1
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.0 // indirect
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.11
	github.com/moby/buildkit v0.6.3
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/selinux v1.4.0 // indirect
	github.com/openllb/doxygen-parser v0.0.0-20200128221307-2aa2d8be1c35
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.4.0
	github.com/urfave/cli/v2 v2.1.1
	github.com/xlab/treeprint v1.0.0
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	google.golang.org/grpc v1.28.0 // indirect
)

replace github.com/alecthomas/participle => github.com/hinshun/participle v0.4.2-0.20200115220927-0afe0602c1fc

replace github.com/moby/buildkit => github.com/hinshun/buildkit v0.0.0-20200410014923-12ce66c4ffa8

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

replace github.com/containerd/containerd => github.com/containerd/containerd v1.3.1-0.20200227195959-4d242818bf55

replace github.com/docker/cli => github.com/docker/cli v0.0.0-20200303162255-7d407207c304

replace github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200227233006-38f52c9fec82

replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.0-pre1.0.20180209125602-c332b6f63c06

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20180806134042-1f13a808da65
