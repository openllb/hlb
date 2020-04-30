This guide will teach you how to setup `hlb` and run a build to output `hello world`.

## Installation

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
	
## Run your first build

Now that you have installed `hlb`, we can run our first build. Typically, we will write our program in a file with a `.hlb` extension, but for our first build we can just pipe the program in from stdin. Try it yourself!

```sh
export BUILDKIT_HOST=docker-container://buildkitd
echo 'fs default() { scratch; mkfile "/output" 0o644 "hello world"; }' | hlb run --download .
```

Once the build has finished, you should end up with a file `output` in your working directory.

```sh
$ cat output
hello world
```

Congratulations! You've now ran your first `hlb` build and downloaded the output back to your system.

!!! tip
	By default, once the build has finished, nothing is exported anywhere. You'll need to specify where the results go, e.g. to your host as a tarball, or pushed to a Docker registry.

Now that we've verified `hlb` is functioning, it's time to start the [tutorial](../tutorial/lets-begin.md).
