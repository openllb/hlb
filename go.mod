module github.com/openllb/hlb

go 1.12

require (
	github.com/alecthomas/participle v1.0.0-alpha2
	github.com/containerd/containerd v1.4.1-0.20201117152358-0edc412565dc
	github.com/creachadair/jrpc2 v0.8.1
	github.com/docker/buildx v0.5.0-rc1
	github.com/docker/cli v20.10.0-beta1.0.20201029214301-1d20b15adc38+incompatible
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v20.10.0-beta1.0.20201110211921-af34b94a78a1+incompatible // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/glycerine/liner v0.0.0-20160121172638-72909af234e0 // indirect
	github.com/go-delve/delve v1.5.0 // indirect
	github.com/google/go-dap v0.3.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/lithammer/dedent v1.1.0
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/go-homedir v1.1.0
	github.com/moby/buildkit v0.8.1-0.20201205083753-0af7b1b9c693
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/openllb/doxygen-parser v0.0.0-20201031162929-e0b5cceb2d0c
	github.com/peterh/liner v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/sourcegraph/go-lsp v0.0.0-20200117082640-b19bb38222e2
	github.com/stretchr/testify v1.5.1
	github.com/tonistiigi/fsutil v0.0.0-20201103201449-0834f99b7b85
	github.com/urfave/cli/v2 v2.1.1
	github.com/xlab/treeprint v1.0.0
	golang.org/x/crypto v0.0.0-20201117144127-c1f2f97bffc9
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	golang.org/x/sys v0.0.0-20201013081832-0aaa2718063a
	google.golang.org/grpc v1.29.1
)

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

// necessary for langserver with vscode
replace github.com/sourcegraph/go-lsp => github.com/radeksimko/go-lsp v0.0.0-20200223162147-9f2c54f29c9f

replace github.com/tonistiigi/fsutil => github.com/slushie/fsutil v0.0.0-20200508061958-7d16a3dcbd1d
