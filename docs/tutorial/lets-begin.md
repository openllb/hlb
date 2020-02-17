Welcome to the `hlb` tutorial!

In this tutorial, we will write a `hlb` program to fetch the `node_modules` of a node project.

Along the way, you will learn the basics of creating and debugging a build graph. If you ever get stuck at any point of the tutorial, feel free to clone [hlb-tutorial]() to get a complete working example.

## Defining a function

Let's start by creating a new directory and a file `node.hlb`. This is where we will write our `hlb` program. We will begin by defining a function which will become our target to build later.

	#!hlb
	fs npmInstall() {
		image "node:alpine"
	}

A function begins with a `return type`, an `identifier`, an optional list of `arguments`, and then followed by a body enclosed in braces. The body must be non-empty, and in this example we are starting from a filesystem of a Docker image `node:alpine`.

Since we haven't executed anything, we aren't done yet. Let's add a few more instructions to complete our program:

	#!hlb
	fs npmInstall() {
		image "node:alpine"
		run "apk add -U git"
		run "git clone https://github.com/left-pad/left-pad.git /src"
		dir "/src"
		run "npm install"
	}

If you are thinking, "Hey, that looks like Dockerfile!", then you would be right! `hlb` is a superset of the features from Dockerfiles, but designed to leverage the full power of BuildKit. Let's go over what we did.

1. Fetched `git` using alpine's package manager `apk`
2. Cloned a simple node project from stash
3. Changed the current working directory to `/src`
4. Run `npm install`, which should produce a directory at `/src/node_modules` containing all the dependencies for the node project

When you are ready, save the `node.hlb` file and run the build by using the `hlb` binary we previously installed.

```sh
hlb run --target npmInstall node.hlb
```

You generated a `node_modules` directory, but since nothing was exported it is still with the BuildKit daemon. Of course, that is what we will be learning next.

## Exporting a directory

Now that our build graph produces a `/src/node_modules`, one thing we might want to do is to export it to our system. However, if we export the target `npmInstall`, we'll not only get the `node_modules` directory, but also the rest of the alpine filesystem. In order to isolate the directory we want, we need to copy it to a new filesystem.

	#!hlb
	fs nodeModules() {
		scratch
		copy npmInstall "/src/node_modules" "/"
	}

As we learned earlier, we can define functions which we can later target when running the build. In this new function, we are starting from a `scratch` filesystem (an empty one), and then copying the `/src/node_modules` from `npmInstall`.

Since `hlb` is a functional language, variables and functions cannot be modified dynamically. When we copy from `npmInstall`, it is always referring to a snapshot of its filesystem after all its instructions have been executed. If we want to modify `npmInstall`, we will have to write a new function that starts from `npmInstall` but it will have to be defined with a new identifier.

Now that we have isolated the directory, we can download the filesystem (containing only the `node_modules`) by specifying `--download <dest-dir>`:

```sh
hlb run --target nodeModules --download . node.hlb
```

After the build have finished, you should see the `node_modules` in your working directory.

```sh
$ ls
node_modules  node.hlb

$ tree node_modules | tail -n 1
307 directories, 4014 files

$ rm -rf node_modules
```

Once you have verified the directory is correct, remove it so we can keep our workspace clean.

!!! tip "Not just to produce images"
	Although we are running a containerized build, it doesn't have to result in a container image. We can leverage the sandboxed environment in containers as a way to have a repeatable workflow. 
	
## Going further

Well done! We've now defined two functions `npmInstall` and `nodeModules` in our `hlb` program, and we've successfully downloaded just the `node_modules` directory to our system. We can still run `npmInstall` independently, because unused nodes in the build graph will be pruned if they're not a dependency of the target.

If you've noticed, we didn't explicitly declare that `npmInstall` must be run before `nodeModules`.  The superpower of `hlb` comes from the implicit build graph constructed by the instructions that invoke other functions. You don't need to think about what is safe to parallelize, and the more you decompose your build into smaller functions, the more concurrent your build!

However, what we achieved so far is also possible with multi-stage Dockerfiles today. In the next chapter, we'll find out about hidden powers in BuildKit which we can start using in `hlb`.
