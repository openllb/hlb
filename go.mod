module github.com/openllb/hlb

go 1.12

require (
	github.com/Microsoft/hcsshim/test v0.0.0-20210428000155-bf20b75af1b9 // indirect
	github.com/alecthomas/participle v1.0.0-alpha2
	github.com/containerd/containerd v1.5.0
	github.com/containerd/fifo v1.0.0 // indirect
	github.com/creachadair/jrpc2 v0.8.1
	github.com/docker/buildx v0.3.2-0.20200410204309-f4ac640252b8
	github.com/docker/cli v20.10.5+incompatible
	github.com/docker/distribution v2.7.1+incompatible
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lithammer/dedent v1.1.0
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-homedir v1.1.0
	github.com/moby/buildkit v0.8.2-0.20210601194713-8df5671844e0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/openllb/doxygen-parser v0.0.0-20201031162929-e0b5cceb2d0c
	github.com/pkg/errors v0.9.1
	github.com/sourcegraph/go-lsp v0.0.0-20200117082640-b19bb38222e2
	github.com/stretchr/testify v1.7.0
	github.com/tonistiigi/fsutil v0.0.0-20210525040343-5dfbf5db66b9
	github.com/urfave/cli/v2 v2.1.1
	github.com/xlab/treeprint v1.0.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	google.golang.org/grpc v1.35.0
)

// includes the changes in github.com/slushie/fsutil v0.0.0-20200508061958-7d16a3dcbd1d
replace github.com/tonistiigi/fsutil => github.com/aaronlehmann/fsutil v0.0.0-20210601195957-d9292d6d3583

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

replace github.com/containerd/containerd => github.com/containerd/containerd v1.4.1-0.20201117152358-0edc412565dc

replace github.com/docker/cli => github.com/docker/cli v0.0.0-20200303162255-7d407207c304

replace github.com/docker/docker => github.com/docker/docker v20.10.0-beta1.0.20201110211921-af34b94a78a1+incompatible

replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.0-pre1.0.20180209125602-c332b6f63c06

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20180806134042-1f13a808da65

// necessary for langserver with vscode
replace github.com/sourcegraph/go-lsp => github.com/radeksimko/go-lsp v0.0.0-20200223162147-9f2c54f29c9f

//replace github.com/tonistiigi/fsutil => github.com/slushie/fsutil v0.0.0-20200508061958-7d16a3dcbd1d
