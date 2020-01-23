module github.com/openllb/hlb

go 1.12

require (
	github.com/alecthomas/participle v0.4.2-0.20191230055107-1fbf95471489
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pretty v0.2.0 // indirect
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.11
	github.com/moby/buildkit v0.6.3
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/image-spec v1.0.1
	github.com/stretchr/testify v1.4.0
	github.com/urfave/cli/v2 v2.1.1
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
)

replace github.com/alecthomas/participle => github.com/hinshun/participle v0.4.2-0.20200115220927-0afe0602c1fc

replace github.com/moby/buildkit => github.com/hinshun/buildkit v0.0.0-20200123030914-aacaae031fb3

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
