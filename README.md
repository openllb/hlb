# hlb

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
# We're still waiting on some upstream PRs to be merged, but soon you'll be able to use standard moby/buildkit
docker run -d --name buildkitd --privileged openllb/buildkit:experimental
```

Then you can run one of the examples in `./examples`:
```sh
export BUILDKIT_HOST=docker-container://buildkitd
hlb run ./examples/node.hlb
```

If your editor has a decent LSP plugin, there is an [Language Server for HLB](https://github.com/openllb/hlb-langserver).
