# hlb

[![GoDoc](https://img.shields.io/static/v1?label=godoc&message=reference&color=blue)](https://pkg.go.dev/github.com/openllb/hlb)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Test](https://github.com/openllb/hlb/workflows/Test/badge.svg)](https://github.com/openllb/hlb/actions?query=workflow%3ATest)

`hlb` is a high-level build language for [BuildKit](https://github.com/moby/buildkit/).

Describe your build in containerized units of work, and BuildKit will build your target as efficiently as possible.

## Getting started with HLB

If you're on a MacOS or Linux (`linux-amd64`), head on over to [Releases](https://github.com/openllb/hlb/releases) to grab a static binary.

Otherwise, you can compile HLB yourself using [go](https://golang.org/dl/):
```sh
go get -u github.com/openllb/hlb/cmd/hlb
```

You'll also need to run `buildkitd` somewhere you can connect to. The easiest way if you have [Docker](https://www.docker.com/get-started), is to run a local buildkit container:
```sh
docker run -d --name buildkitd --privileged moby/buildkit:master
```

Then you can run one of the examples in `./examples`:
```sh
export BUILDKIT_HOST=docker-container://buildkitd
hlb run ./examples/node.hlb
```

If your editor has a decent LSP plugin, there is an [Language Server for HLB](https://github.com/openllb/hlb-langserver).
