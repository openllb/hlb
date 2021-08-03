module github.com/openllb/hlb

go 1.16

require (
	github.com/alecthomas/participle v1.0.0-alpha2
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.6.0-beta.2.0.20211112054404-aa1b0736165c
	github.com/creachadair/jrpc2 v0.26.1
	github.com/docker/buildx v0.5.1
	github.com/docker/cli v20.10.8+incompatible
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/libnetwork v0.8.0-dev.2.0.20210525090646-64b7a4574d14 // indirect
	github.com/fvbommel/sortorder v1.0.2 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lithammer/dedent v1.1.0
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-homedir v1.1.0
	github.com/moby/buildkit v0.9.1-0.20211118220822-8d02e473a494
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20210819154149-5ad6f50d6283
	github.com/openllb/doxygen-parser v0.0.0-20201031162929-e0b5cceb2d0c
	github.com/pkg/errors v0.9.1
	github.com/sourcegraph/go-lsp v0.0.0-20200117082640-b19bb38222e2
	github.com/stretchr/testify v1.7.0
	github.com/theupdateframework/notary v0.7.0 // indirect
	github.com/tonistiigi/fsutil v0.0.0-20210818161904-4442383b5028
	github.com/urfave/cli/v2 v2.1.1
	github.com/xlab/treeprint v1.0.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211025201205-69cdffdb9359
	google.golang.org/grpc v1.41.0
)

// includes the changes in github.com/slushie/fsutil v0.0.0-20200508061958-7d16a3dcbd1d
replace github.com/tonistiigi/fsutil => github.com/aaronlehmann/fsutil v0.0.0-20210601195957-d9292d6d3583

// necessary for langserver with vscode
replace github.com/sourcegraph/go-lsp => github.com/radeksimko/go-lsp v0.0.0-20200223162147-9f2c54f29c9f
