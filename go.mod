module github.com/openllb/hlb

go 1.12

require (
	github.com/alecthomas/participle v1.0.0-alpha1.0.20201031050245-4435aeea334f
	github.com/containerd/containerd v1.4.0-beta.2.0.20200728183644-eb6354a11860
	github.com/creachadair/jrpc2 v0.8.1
	github.com/docker/buildx v0.3.2-0.20200410204309-f4ac640252b8
	github.com/docker/cli v0.0.0-20200227165822-2298e6a3fe24
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lithammer/dedent v1.1.0
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.11
	github.com/mitchellh/go-homedir v1.1.0
	github.com/moby/buildkit v0.7.1-0.20200806195445-545532ab0e75
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/openllb/doxygen-parser v0.0.0-20201031162929-e0b5cceb2d0c
	github.com/pkg/errors v0.9.1
	github.com/sourcegraph/go-lsp v0.0.0-20200117082640-b19bb38222e2
	github.com/stretchr/testify v1.5.1
	github.com/tonistiigi/fsutil v0.0.0-20200724193237-c3ed55f3b481
	github.com/urfave/cli/v2 v2.1.1
	github.com/xlab/treeprint v1.0.0
	golang.org/x/crypto v0.0.0-20200221231518-2aa609cf4a9d
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	google.golang.org/grpc v1.28.0
)

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

replace github.com/containerd/containerd => github.com/containerd/containerd v1.4.0-beta.2.0.20200728183644-eb6354a11860

replace github.com/docker/cli => github.com/docker/cli v0.0.0-20200303162255-7d407207c304

replace github.com/docker/docker => github.com/docker/docker v17.12.0-ce-rc1.0.20200227233006-38f52c9fec82+incompatible

replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.0-pre1.0.20180209125602-c332b6f63c06

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20180806134042-1f13a808da65

// necessary for langserver with vscode
replace github.com/sourcegraph/go-lsp => github.com/radeksimko/go-lsp v0.0.0-20200223162147-9f2c54f29c9f

replace github.com/tonistiigi/fsutil => github.com/slushie/fsutil v0.0.0-20200508061958-7d16a3dcbd1d
