module github.com/openllb/hlb

go 1.16

require (
	github.com/alecthomas/participle/v2 v2.0.0-alpha7.0.20211230082035-5a357f57e525
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/containerd/console v1.0.3
	github.com/containerd/containerd v1.6.14
	github.com/creachadair/jrpc2 v0.26.1
	github.com/creack/pty v1.1.11
	github.com/docker/buildx v0.10.0
	github.com/docker/cli v23.0.0-rc.1+incompatible
	github.com/docker/distribution v2.8.1+incompatible
	github.com/docker/docker v23.0.0-rc.1+incompatible
	github.com/fvbommel/sortorder v1.0.2 // indirect
	github.com/google/go-dap v0.6.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lithammer/dedent v1.1.0
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.14
	github.com/moby/buildkit v0.11.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0-rc2
	github.com/openllb/doxygen-parser v0.0.0-20201031162929-e0b5cceb2d0c
	github.com/pkg/errors v0.9.1
	github.com/sourcegraph/go-lsp v0.0.0-20200117082640-b19bb38222e2
	github.com/stretchr/testify v1.8.1
	github.com/theupdateframework/notary v0.7.0 // indirect
	github.com/tonistiigi/fsutil v0.0.0-20230105215944-fb433841cbfa
	github.com/urfave/cli/v2 v2.3.0
	github.com/xlab/treeprint v1.0.0
	golang.org/x/crypto v0.2.0
	golang.org/x/sync v0.1.0
	golang.org/x/sys v0.3.0
	google.golang.org/grpc v1.50.1
)

replace github.com/sourcegraph/go-lsp => github.com/radeksimko/go-lsp v0.0.0-20200223162147-9f2c54f29c9f
