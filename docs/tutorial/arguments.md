At the end of the previous chapter, we were left with this:

	#!hlb
	fs src() {
		git "https://github.com/left-pad/left-pad.git" "master"
	}
	
	fs npmInstall() {
		image "node:alpine"
		dir "/src"
		run "npm install" with option {
			mount src "/src"
			mount fs { scratch; } "/src/node_modules" as nodeModules
		}
	}

Currently our git repository is hardcoded into our program. Let's refactor it out so we can build `nodeModules` for other node projects.

## Refactoring our program

When considering what to change to variables, we want to aim for extensibility. One approach may be to change the git remote into a variable, but that would mean we're stuck with git sources. Instead, let's refactor `src` to `npmInstall`'s function arguments so that we are opaque to where the source comes from.

	#!hlb
	fs npmInstall(fs src) {
		image "node:alpine"
		dir "/src"
		run "npm install" with option {
			mount src "/src"
			mount fs { scratch; } "/src/node_modules" as nodeModules
		}
	}
	
Great! Now we can write a new function named `remoteModules` to pass in our git source. However, we don't want to invoke `npmInstall` because that will return the alpine filesystem, what we rather want is `nodeModules`.

When an alias is declared in the body of a `fs` block, it also inherits the signature of the parent function. This is because `nodeModules` depends on the value of `src`, so when we defined a signature for `npmInstall`, `nodeModules` also inherited the signature:

	#!hlb
	fs nodeModules(fs src)

We know how to invoke `nodeModules`, so let's pass the same git source we used earlier.
	
	#!hlb
	fs remoteModules() {
		nodeModules fs { git "https://github.com/left-pad/left-pad.git" "master"; }
	}
	
There we have it. A reusable `nodeModules` function that is opaque to where the source code comes from. Let's take a look at one more type of source filesystem.

## Local sources

So far we have been dealing with only remote sources like `image` and `git`, but what if you wanted to provide your working directory as a source filesystem?

Turns out there is a source `local` that provides just that ability. Here's the signature:

	#!hlb
	# Rsyncs a local directory at path to a scratch filesystem.
	fs local(string path)
	
We don't have a local node project at the moment, so let's write a function to initialize a node project and add `left-pad` as a dependency. We learnt how to use arguments just now, so let's apply our learnings and write a generic function.

	#!hlb
	fs npmInit(string package) {
		image "node:alpine"
		dir "/src"
		run string {
			format "npm init -y && npm install --package-lock-only %s" package
		} with option {
			mount fs { scratch; } "/src" as nodeProject
		}
	}
	
	fs nodeProjectWithLeftPad() {
		nodeProject "left-pad"
	}

This time, instead of passing a string literal, we can pass a string block literal where we have access to string functions like `format`. This allows us to interpolate values into string to install an arbitrary package.

When you're ready, run a build targetting `nodeProjectWithLeftPad` and download the initialized node project.

```sh
./hlb run --target nodeProjectWithLeftPad --download . node.hlb
```

You should see two new files `package.json` and `package-lock.json` in your working directory.

```sh
$ ls
node.hlb  package.json  package-lock.json
```

Now we can use the `local` source to download `node_modules`, but let's also use a `includePatterns` option to specify exactly what files we should sync up.

	#!hlb
	fs localModules() {
		nodeModules fs {
			local "." with option {
				includePatterns "package.json" "package-lock.json"
			}
		}
	}

And finally, we can run `npm install` remotely using our working directory and transfer back the `node_modules`.
	
```sh
./hlb run --target localModules --download node_modules node.hlb
```

## Advanced concepts

You've made it to the end of basic concepts! Throughout the last few chapters, you wrote a few `hlb` programs to run `npm install` downloaded just the `node_modules` directory back to your system. You refactored it so that the function can be opaque to what the source comes from, and then you provided your working directory as a source filesystem.

Next up, you can start writing your own `hlb` programs with the help of the [Reference](../reference.md).

As your build graphs grow in complexity, it will be crucial to be able to introspect and diagonose issues as they come up. The next chapter will walk through the debugger that comes with the `hlb` CLI.
