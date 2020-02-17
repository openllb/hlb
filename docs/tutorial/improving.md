In the previous chapter, we wrote our first `hlb` program to install the dependencies of a node project. It currently looks like this:

	#!hlb
	fs npmInstall() {
		image "node:alpine"
		run "apk add -U git"
		run "git clone https://github.com/left-pad/left-pad.git /src"
		dir "/src"
		run "npm install"
	}
	
	fs nodeModules() {
		scratch
		copy npmInstall "/src/node_modules" "/"
	}
	
If we change to a bigger project, copying over the `node_modules` to a scratch filesystem only to isolate the directory is pretty expensive. In this chapter, we'll learn about techniques to improve our build which isn't available through the Dockerfile frontend.

## Removing the unnecessary copy

Instructions like `image`, `run`, and `copy` are also regular functions, and you must invoke them with the required arguments. But some builtin functions have optional features that can be accessed in an option block. For example, `run` has an optional function `mount` that temporarily mounts a filesystem to a `mountpoint` directory while `run` is executing.

The builtin `mount` function has the following signature:

	#!hlb
	# Mounts a source filesystem to the specific mountpoint.
	fs mount(fs source, string mountpoint)

We can apply options to the `run` function by adding `with <option>` after the arguments, where `option` can be a function or a block literal. Let's first take a look at block literals.

	#!hlb
	# A block literal that returns type `option`.
	option {
		mount foo "/foo"
		mount bar "/bar"
	}
	
	# An inline block literal that returns type `fs`.
	fs { scratch; }
	
	option {
		# Mount taking a `fs` block literal as the source filesystem.
		mount fs { scratch; } "/output"
	}
	
	
Block literals cannot be defined in the global scope, but you can define them where an argument is expected. When the statements within a block are all in one line, each statement must be suffixed with a `;` as a delimiter. In the final example above, we defined an option block literal that mounts a scratch filesystem into `/output`, ready to receive output files. Handy!

Now with this new ability, let's avoid the unnecessary `copy`!

	#!hlb
	fs npmInstall() {
		image "node:alpine"
		run "apk add -U git"
		run "git clone https://github.com/left-pad/left-pad.git /src"
		dir "/src"
		run "npm install" with option {
			mount fs { scratch; } "/src/node_modules"
		}
	}
	
This time, when `npm install` runs, it will write the files directly into the scratch filesystem we mounted to `/src/node_modules`. Hooray!

But wait, if we run the build targeting `npmInstall`, that will still give us the alpine filesystem. Not only that, once `npm install` has finished, the scratch filesystem will be unmounted leaving behind no `node_modules` directory at all! Haven't we made things worse?

Fear not, for there is still one last concept to introduce!

## Defining aliases

Currently, the only targets we can run are functions we defined in the global scope. We can also target statements inside the body of a function by defining an alias.

After the arguments to the function and the optional `with <option>` block, you can add `as <identifier>` to define an alias for the filesystem at that step. Usually options are not allowed to be aliased but `mount` is an exception.

	#!hlb
	fs npmInstall() {
		image "node:alpine"
		run "apk add -U git"
		run "git clone https://github.com/left-pad/left-pad.git /src"
		dir "/src"
		run "npm install" with option {
			mount fs { scratch; } "/src/node_modules" as nodeModules
		}
	}

Try running a build targetting `nodeModules` now, and this time we don't have to download it.

```sh
./hlb run --target nodeModules node.hlb
```

Your build should complete slightly quicker (or much quicker if you had more dependencies), but we don't have to stop there. We briefly mentioned in the previous chapter that we chose to start from a filesystem of a Docker image, so we can explore other source functions too.

## Git sources

Before `npm install` happens, we need to clone the repository, and before that happens we need to download the `node:alpine` image. However, cloning the repository doesn't strictly depend on the `node:alpine` image. What if we could pull the image and clone the repository concurrently?

Using the `mount` function we just learnt, we can define a new `fs` function that clones the repository and mount it. But that will still require another image. Luckily for us there is a `git` builtin function that can efficiently prepare a filesystem containing a `git` repository! Here's the signature:

	#!hlb
	# Creates a scratch filesystem with a git repostory from remote checked out at
	# a specified branch, commit or tag.
	fs git(string remote, string ref)

Instead of cloning the repository and then running it, we can implicitly depend on a function that checkouts our repository.

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
	
Run the build again to see the speed improve once again.

```sh
./hlb run --target npmInstall node.hlb
```

!!! tip "Formatting"
	Consistency helps with readability, so `hlb` comes with a formatter so that programs will look consistently formatted. Simply run `hlb format -w node.hlb` and it will format your file for you.

## Recap

At the end of two chapters, we have wrote our first `hlb` program and optimized it by writing `node_modules` into a mount. Then we improved it a little more by leveraging a `git` source, allowing the clone to happen concurrently with the pull of the `node:alpine` image.

You may have noticed that all this time, all our functions we defined had no arguments. In the next chapter, we'll refactor our example to make our program more generic.
