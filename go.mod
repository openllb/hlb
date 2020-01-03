module github.com/openllb/hlb

go 1.12

require (
	github.com/alecthomas/participle v0.4.2-0.20191230055107-1fbf95471489
	github.com/logrusorgru/aurora v0.0.0-20191116043053-66b7ad493a23
	github.com/mattn/go-isatty v0.0.11
	github.com/moby/buildkit v0.6.3
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/stretchr/testify v1.4.0
	github.com/urfave/cli/v2 v2.1.1
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
)

replace github.com/alecthomas/participle => github.com/hinshun/participle v0.4.2-0.20200102003105-2da58af4b8ee

replace github.com/moby/buildkit => github.com/hinshun/buildkit v0.0.0-20191220011919-e571a1d83df9
