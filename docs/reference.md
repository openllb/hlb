## <span class='hlb-type'>fs</span> functions
### <span class='hlb-type'>fs</span> <span class='hlb-name'>copy</span>(<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>dst</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>"
	the filesystem to copy from.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>"
	the path from the input filesystem.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>dst</span>"
	the path in the current filesystem.

Copies a file from an input filesystem into the current filesystem.

	#!hlb
	fs default() {
		copy scratch "src" "dst" with option {
			allowEmptyWildcard
			allowWildcard
			chmod 0
			chown "owner"
			contentsOnly
			createDestPath
			createdTime "created"
			followSymlinks
			unpack
		}
	}


#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>allowEmptyWildcard</span>()


Allows wildcards to match no files in the path to copy.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>allowWildcard</span>()


Allows wildcards in the path to copy.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>chmod</span>(<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	the new permissions of the file.

Modifies the permissions of the copied files.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>chown</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	the user:group owner of the copy path.

Change the owner of the copy path.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>contentsOnly</span>()


If the &#x60;src&#x60; path is a directory, only the contents of the directory is
copied to the destination.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>createDestPath</span>()


Create the parent directories of the destination if they don&#x27;t already exist.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>createdTime</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	the created time in the RFC3339 format.

Sets the created time of the copy path.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>followSymlinks</span>()


Follow symlinks in the input filesystem and copy the symlink targets too.

#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>unpack</span>()


If the &#x60;src&#x60; path is an archive, attempt to unpack its contents into the
destination.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>dir</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the new working directory.

Sets the working directory for all subsequent calls in this filesystem block.

	#!hlb
	fs default() {
		dir "path"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>dockerLoad</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	the name of the Docker image.

Loads the filesystem as a Docker image to the docker client found in your
environment.

	#!hlb
	fs default() {
		dockerLoad "ref"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>dockerPush</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	a distribution reference. if not fully qualified, it will be expanded the same as the docker CLI.

Pushes the filesystem to a registry following the distribution
spec: https://github.com/opencontainers/distribution-spec/

	#!hlb
	fs default() {
		dockerPush "ref"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>download</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	the destination filepath for the filesystem contents.

Downloads the filesystem to a local path.

	#!hlb
	fs default() {
		download "localPath"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>downloadDockerTarball</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	the destination filepath for the tarball.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	the name of the Docker image.

Downloads the filesystem as a Docker image tarball to a local path.
The tarball is able to be loaded into a docker engine via &#x60;docker load&#x60;.
See: https://docs.docker.com/engine/reference/commandline/save/
and https://docs.docker.com/engine/reference/commandline/load/

	#!hlb
	fs default() {
		downloadDockerTarball "localPath" "ref"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>downloadOCITarball</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	the destination filepath for the tarball.

Downloads the filesystem as a OCI filesystem bundle to a local path.
See: https://github.com/opencontainers/runtime-spec/blob/master/bundle.md

	#!hlb
	fs default() {
		downloadOCITarball "localPath"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>downloadTarball</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	the destination filepath for the tarball.

Downloads the filesystem as a tarball to a local path.

	#!hlb
	fs default() {
		downloadTarball "localPath"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>env</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	the environment key.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	the environment value.

Sets an environment key pair for all subsequent calls in this filesystem
block.

	#!hlb
	fs default() {
		env "key" "value"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>frontend</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>source</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>source</span>"
	

Generates a filesystem using an external frontend.

	#!hlb
	fs default() {
		frontend "source" with option {
			input "key" scratch
			opt "key" "value"
		}
	}


#### <span class='hlb-type'>option::frontend</span> <span class='hlb-name'>input</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>fs</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	an unique key for the input.
!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>value</span>"
	a filesystem as an input.

Provide an input filesystem to the external frontend. Read the documentation
for the frontend to see what it will accept.

#### <span class='hlb-type'>option::frontend</span> <span class='hlb-name'>opt</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	an unique key for the option.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	a value for the option.

Provide a key value pair to the external frontend. Read the documentation
for the frontend to see what it will accept.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>git</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>remote</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>remote</span>"
	the fully qualified git remote.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	the git reference to check out.

A filesystem with the files from a git repository checked out from
a git reference. Note that by default, the &#x60;.git&#x60; directory is not included.

	#!hlb
	fs default() {
		git "remote" "ref" with option {
			keepGitDir
		}
	}


#### <span class='hlb-type'>option::git</span> <span class='hlb-name'>keepGitDir</span>()


Keeps the &#x60;.git&#x60; directory of the git repository.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>http</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>url</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>url</span>"
	a fully-qualified URL to send a HTTP GET request.

A filesystem with a file retrieved from a HTTP URL.

	#!hlb
	fs default() {
		http "url" with option {
			checksum "digest"
			chmod 0
			filename "name"
		}
	}


#### <span class='hlb-type'>option::http</span> <span class='hlb-name'>checksum</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>digest</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>digest</span>"
	a checksum in the form of an OCI digest. https://github.com/opencontainers/image-spec/blob/master/descriptor.md#digests

Verifies the checksum of the retrieved file against a digest.

#### <span class='hlb-type'>option::http</span> <span class='hlb-name'>chmod</span>(<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	the new permissions of the file.

Modifies the permissions of the retrieved file.

#### <span class='hlb-type'>option::http</span> <span class='hlb-name'>filename</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the file.

Writes the retrieved file with a specified name.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>image</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	a docker registry reference. if not fully qualified, it will be expanded the same as the docker CLI.

An OCI image&#x27;s filesystem.

	#!hlb
	fs default() {
		image "ref" with option {
			resolve
		}
	}


#### <span class='hlb-type'>option::image</span> <span class='hlb-name'>resolve</span>()


Resolves the OCI Image Config and inherit its environment, working directory,
and entrypoint.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>local</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the local path to the directory to sync up.

A filesystem with the files synced up from a directory on the
local system.

	#!hlb
	fs default() {
		local "path" with option {
			excludePatterns "pattern"
			followPaths "path"
			includePatterns "pattern"
		}
	}


#### <span class='hlb-type'>option::local</span> <span class='hlb-name'>excludePatterns</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	a list of patterns for files that should not be synced.

Sync only files that do not match any of the excluded patterns.

#### <span class='hlb-type'>option::local</span> <span class='hlb-name'>followPaths</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	a list of paths to files that may be symlinks.

Sync the targets of symlinks if path is to a symlink.

#### <span class='hlb-type'>option::local</span> <span class='hlb-name'>includePatterns</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	a list of patterns for files that should be synced.

Sync only files that match any of the included patterns.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>mkdir</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>, <span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path of the directory.
!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	the permissions of the directory.

Creates a directory in the current filesystem.

	#!hlb
	fs default() {
		mkdir "path" 0 with option {
			chown "owner"
			createParents
			createdTime "created"
		}
	}


#### <span class='hlb-type'>option::mkdir</span> <span class='hlb-name'>chown</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	the user:group owner of the directory.

Change the owner of the directory.

#### <span class='hlb-type'>option::mkdir</span> <span class='hlb-name'>createParents</span>()


Create the parent directories if they don&#x27;t exist already.

#### <span class='hlb-type'>option::mkdir</span> <span class='hlb-name'>createdTime</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	the created time in the RFC3339 format.

Sets the created time of the directory.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>mkfile</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>, <span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>content</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path of the file.
!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	the permissions of the file.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>content</span>"
	the contents of the file.

Creates a file in the current filesystem.

	#!hlb
	fs default() {
		mkfile "path" 0 "content" with option {
			chown "owner"
			createdTime "created"
		}
	}


#### <span class='hlb-type'>option::mkfile</span> <span class='hlb-name'>chown</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	the user:group owner of the file.

Change the owner of the file.

#### <span class='hlb-type'>option::mkfile</span> <span class='hlb-name'>createdTime</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	the created time in the RFC3339 format.

Sets the created time of the file.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>rm</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path of the file to remove.

Removes a file from the current filesystem.

	#!hlb
	fs default() {
		rm "path" with option {
			allowNotFound
			allowWildcard
		}
	}


#### <span class='hlb-type'>option::rm</span> <span class='hlb-name'>allowNotFound</span>()


Allows the file to not be found.

#### <span class='hlb-type'>option::rm</span> <span class='hlb-name'>allowWildcard</span>()


Allows wildcards in the path to remove.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>run</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>"
	are optional arguments to execute.

Executes an command in the current filesystem.
If no arguments are given, it will execute the current args set on the
filesystem.
If exactly one arg is given it will be wrapped with /bin/sh -c &#x27;arg&#x27;.
If more than one arg is given, it will be executed directly, without a shell.

	#!hlb
	fs default() {
		run "arg" with option {
			capture
			dir "path"
			env "key" "value"
			forward "src" "dest"
			host "hostname" "address"
			ignoreCache
			mount scratch "mountPoint"
			network "networkmode"
			readonlyRootfs
			secret "localPath" "mountPoint"
			security "securitymode"
			shlex
			ssh
			user "name"
		}
	}


#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>capture</span>()




#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>dir</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the new working directory.

Sets the working directory for the duration of the run command.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>env</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	the environment key.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	the environment value.

Sets an environment key pair for the duration of the run command.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>forward</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>dest</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>"
	a fully qualified URI to forward traffic to/from.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>dest</span>"
	a mountpoint for a unix domain socket that is forwarded to/from.

Forwards traffic to/from a local source to a unix domain socket mounted for
the duration of the run command. The source must be a fully qualified URI
where the scheme must be either &#x60;unix://&#x60; or &#x60;tcp://&#x60;.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>host</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>hostname</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>address</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>hostname</span>"
	the host name of the entry, may include spaces to delimit multiple host names.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>address</span>"
	the IP of the entry.

Adds a host entry to /etc/hosts for the duration of the run command.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>ignoreCache</span>()


Ignore any previously cached results for the run command.
@ return an option to ignore existing cache for the run command.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>mount</span>(<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>"
	the additional filesystem to mount. the input&#x27;s root filesystem becomes available from the mountPoint directory.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	the directory where the mount is attached.

Attaches an additional filesystem for the duration of the run command.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>network</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>networkmode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>networkmode</span>"
	the network mode of the container, must be one of the following: - unset: use the default network provider. - host: use the host&#x27;s network namespace. - none: disable networking.

Sets the networking mode for the duration of the run command. By default, the
value is &#x60;unset&#x60; (using BuildKit&#x27;s CNI provider, otherwise its host
namespace).

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>readonlyRootfs</span>()


Sets the rootfs as read-only for the duration of the run command.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>secret</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	the filepath for a secure file or directory.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	the directory where the secret is attached.

Mounts a secure file for the duration of the run command. Secrets are
attached via a tmpfs mount, so all the data stays in volatile memory.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>security</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>securitymode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>securitymode</span>"
	the security mode of the container, must be one of the following: - sandbox: use the default containerd seccomp profile. - insecure: enables all capabilities.

Sets the security mode for the duration of the run command. By default, the
value is &#x60;sandbox&#x60;.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>shlex</span>()


Attempt to lex the single-argument shell command provided to &#x60;run&#x60;
to determine if a &#x60;/bin/sh -c &#x27;...&#x27;&#x60; wrapper needs to be added.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>ssh</span>()


Mounts a SSH socket for the duration of the run command. By default, it will
try to use the SSH socket found from $SSH_AUTH_SOCK. Otherwise, an option
&#x60;localPath&#x60; can be provided to specify a filepath to a SSH auth socket or
*.pem file.

#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>user</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the user.

Sets the current user for the duration of the run command.


### <span class='hlb-type'>fs</span> <span class='hlb-name'>scratch</span>()


An empty filesystem.

	#!hlb
	fs default() {
		scratch
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>shell</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>"
	the list of args used to prefix &#x60;run&#x60; statements.

Sets the current shell command to use when executing subsequent &#x60;run&#x60;
methods. By default, this is [&quot;sh&quot;, &quot;-c&quot;].

	#!hlb
	fs default() {
		shell "arg"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>user</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the user.

Sets the current user for all subsequent calls in this filesystem block.

	#!hlb
	fs default() {
		user "name"
	}



## <span class='hlb-type'>string</span> functions
### <span class='hlb-type'>string</span> <span class='hlb-name'>format</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>formatString</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>values</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>formatString</span>"
	the format specifier.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>values</span>"
	the list of values to be interpolated into the format specifier.

A format specifier that is interpolated with values.

	#!hlb
	string myString() {
		format "formatString" "values"
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localArch</span>()


The architecture for the clients local environment.

	#!hlb
	string myString() {
		localArch
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localCwd</span>()


The current working directory from the clients local environment.

	#!hlb
	string myString() {
		localCwd
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localEnv</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	the environment variable&#x27;s key.

An environment variable from the client&#x27;s local environment.

	#!hlb
	string myString() {
		localEnv "key"
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localOs</span>()


The OS from the clients local environment.

	#!hlb
	string myString() {
		localOs
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localRun</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>"
	a command to execute.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>"
	optional arguments to the command.

Executes an command in the local environment.
If exactly one arg is given it will be wrapped with /bin/sh -c &#x27;arg&#x27;.
If more than one arg is given, it will be executed directly, without a shell.

	#!hlb
	string myString() {
		localRun "command" "args" with option {
			ignoreError
			includeStderr
			onlyStderr
			shlex
		}
	}


#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>ignoreError</span>()


If the command returns a non-zero status code ignore
the failure and continue processing the hlb file.

#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>includeStderr</span>()


Capture stderr intermixed with stdout on the command.

#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>onlyStderr</span>()


Only capture the stderr from the command, ignore stdout.

#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>shlex</span>()


Attempt to lex the single-argument shell command provided to &#x60;localRun&#x60;
to determine if a &#x60;/bin/sh -c &#x27;...&#x27;&#x60; wrapper needs to be added.


### <span class='hlb-type'>string</span> <span class='hlb-name'>template</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>text</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>text</span>"
	the text of the template

Process text as a Go text template.
For template syntax documentation see:
https://golang.org/pkg/text/template/

	#!hlb
	string myString() {
		template "text" with option {
			stringField "name" "value"
		}
	}


#### <span class='hlb-type'>option::template</span> <span class='hlb-name'>stringField</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the field inside the template
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	the value of the field inside the template

Add a string field with provided name to be available
inside the template.



<style>
.hlb-type {
	color: #d73a49
}

.hlb-variable {
	color: #0366d6
}

.hlb-name {
	font-weight: bold;
}
</style>
