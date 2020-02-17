## Functions
### <span class='hlb-type'>fs</span> generate(<span class='hlb-type'>fs</span> <span class='hlb-variable'>frontend</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>frontend</span>"
	a filesystem with an executable that runs a BuildKit gateway GRPC client over stdio.

Generates a filesystem using an external frontend.

	#!hlb
	fs default() {
		generate fs { scratch; } with option {
			frontendInput "key" fs { scratch; }
			frontendOpt "key" "value"
		}
	}


#### <span class='hlb-type'>option::generate</span> frontendInput(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>fs</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	an unique key for the input.
!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>value</span>"
	a filesystem as an input.

Provide an input filesystem to the external frontend. Read the documentation
for the frontend to see what it will accept.

#### <span class='hlb-type'>option::generate</span> frontendOpt(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	an unique key for the option.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	a value for the option.

Provide a key value pair to the external frontend. Read the documentation
for the frontend to see what it will accept.


### <span class='hlb-type'>fs</span> git(<span class='hlb-type'>string</span> <span class='hlb-variable'>remote</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

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


#### <span class='hlb-type'>option::git</span> keepGitDir()


Keeps the &#x60;.git&#x60; directory of the git repository.


### <span class='hlb-type'>fs</span> http(<span class='hlb-type'>string</span> <span class='hlb-variable'>url</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>url</span>"
	a fully-qualified URL to send a HTTP GET request.

A filesystem with a file retrieved from a HTTP URL.

	#!hlb
	fs default() {
		http "url" with option {
			checksum "digest"
			chmod 0644
			filename "name"
		}
	}


#### <span class='hlb-type'>option::http</span> checksum(<span class='hlb-type'>string</span> <span class='hlb-variable'>digest</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>digest</span>"
	a checksum in the form of an OCI digest. https://github.com/opencontainers/image-spec/blob/master/descriptor.md#digests

Verifies the checksum of the retrieved file against a digest.

#### <span class='hlb-type'>option::http</span> chmod(<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>"
	the new permissions of the file in octal.

Modifies the permissions of the retrieved file.

#### <span class='hlb-type'>option::http</span> filename(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the file.

Writes the retrieved file with a specified name.


### <span class='hlb-type'>fs</span> image(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	a docker registry reference. if not fully qualified, it will be expanded the same as the docker CLI.

An OCI image&#x27;s filesystem.

	#!hlb
	fs default() {
		image "ref" with option {
			resolve
		}
	}


#### <span class='hlb-type'>option::image</span> resolve()


Resolves the OCI Image Config and inherit its environment, working directory,
and entrypoint.


### <span class='hlb-type'>fs</span> local(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

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


#### <span class='hlb-type'>option::local</span> excludePatterns(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	a list of patterns for files that should not be synced.

Sync only files that do not match any of the excluded patterns.

#### <span class='hlb-type'>option::local</span> followPaths(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	a list of paths to files that may be symlinks.

Sync the targets of symlinks if path is to a symlink.

#### <span class='hlb-type'>option::local</span> includePatterns(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	a list of patterns for files that should be synced.

Sync only files that match any of the included patterns.


### <span class='hlb-type'>fs</span> scratch()


An empty filesystem.

	#!hlb
	fs default() {
		scratch
	}




## Methods
### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) copy(<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>dst</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>"
	the filesystem to copy from.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>"
	the path from the input filesystem.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>dst</span>"
	the path in the current filesystem.

Copies a file from an input filesystem into the current filesystem.

	#!hlb
	fs default() {
		scratch
		copy fs { scratch; } "src" "dst" with option {
			contentsOnly
			createDestPath
			followSymlinks
			unpack
		}
	}

#### <span class='hlb-type'>option::copy</span> contentsOnly()


If the &#x60;src&#x60; path is a directory, only the contents of the directory is
copied to the destination.



#### <span class='hlb-type'>option::copy</span> createDestPath()


Create the parent directories of the destination if they don&#x27;t already exist.



#### <span class='hlb-type'>option::copy</span> followSymlinks()


Follow symlinks in the input filesystem and copy the symlink targets too.



#### <span class='hlb-type'>option::copy</span> unpack()


If the &#x60;src&#x60; path is an archive, attempt to unpack its contents into the
destination.




### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) dir(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the new working directory.

Sets the working directory for all subsequent calls in this filesystem block.

	#!hlb
	fs default() {
		scratch
		dir "path"
	}


### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) env(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	the environment key.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	the environment value.

Sets an environment key pair for all subsequent calls in this filesystem
block.

	#!hlb
	fs default() {
		scratch
		env "key" "value"
	}


### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) mkdir(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>, <span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path of the directory.
!!! info "<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>"
	the permissions of the directory.

Creates a directory in the current filesystem.

	#!hlb
	fs default() {
		scratch
		mkdir "path" 0644 with option {
			chown "owner"
			createParents
			createdTime "created"
		}
	}

#### <span class='hlb-type'>option::mkdir</span> chown(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	the user:group owner of the directory.

Change the owner of the directory.



#### <span class='hlb-type'>option::mkdir</span> createParents()


Create the parent directories if they don&#x27;t exist already.



#### <span class='hlb-type'>option::mkdir</span> createdTime(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	the created time in the RFC3339 format.

Sets the created time of the directory.




### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) mkfile(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>, <span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>content</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path of the file.
!!! info "<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>"
	the permissions of the file.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>content</span>"
	the contents of the file.

Creates a file in the current filesystem.

	#!hlb
	fs default() {
		scratch
		mkfile "path" 0644 "content" with option {
			chown "owner"
			createdTime "created"
		}
	}

#### <span class='hlb-type'>option::mkfile</span> chown(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	the user:group owner of the file.

Change the owner of the file.



#### <span class='hlb-type'>option::mkfile</span> createdTime(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	the created time in the RFC3339 format.

Sets the created time of the file.




### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) rm(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path of the file to remove.

Removes a file from the current filesystem.

	#!hlb
	fs default() {
		scratch
		rm "path" with option {
			allowNotFound
			allowWildcards
		}
	}

#### <span class='hlb-type'>option::rm</span> allowNotFound()


Allows the file to not be found.



#### <span class='hlb-type'>option::rm</span> allowWildcards()


Allows wildcards in the path to remove.




### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) run(<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>"
	a command to execute.

Executes an command in the current filesystem.
If no arguments are given, it will execute the current args set on the
filesystem.
If exactly one arg is given, it will attempt to parse as a command with
shell-style quoting. If it remains a single element, it is executed directly,
otherwise it is run with the current shell.
If more than one arg is given, it will be executed directly, without a shell.

	#!hlb
	fs default() {
		scratch
		run "arg" with option {
			dir "path"
			env "key" "value"
			host "hostname" "address"
			mount fs { scratch; } "mountPoint" with option {
				readonly
				tmpfs
				sourcePath "path"
				cache "cacheid" "sharingmode"
			}
			network "networkmode"
			readonlyRootfs
			secret "localPath" "mountPoint" with option {
				uid 0
				gid 0
				mode 0644
			}
			security "securitymode"
			ssh with option {
				target "mountPoint"
				localPath "path"
				uid 0
				gid 0
				mode 0644
			}
			user "name"
		}
	}

#### <span class='hlb-type'>option::run</span> dir(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the new working directory.

Sets the working directory for the duration of the run command.



#### <span class='hlb-type'>option::run</span> env(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	the environment key.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	the environment value.

Sets an environment key pair for the duration of the run command.



#### <span class='hlb-type'>option::run</span> host(<span class='hlb-type'>string</span> <span class='hlb-variable'>hostname</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>address</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>hostname</span>"
	the host name of the entry, may include spaces to delimit multiple host names.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>address</span>"
	the IP of the entry.

Adds a host entry to /etc/hosts for the duration of the run command.



#### <span class='hlb-type'>option::run</span> mount(<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>"
	the additional filesystem to mount. the input&#x27;s root filesystem becomes available from the mountPoint directory.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	the directory where the mount is attached.

Attaches an additional filesystem for the duration of the run command.


#### <span class='hlb-type'>option::mount</span> readonly()


Sets the mount to be attached as a read-only filesystem.

#### <span class='hlb-type'>option::mount</span> tmpfs()


Sets the mount to be attached as a tmpfs filesystem.

#### <span class='hlb-type'>option::mount</span> sourcePath(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path in the input filesystem.

Mount a path from the input filesystem. By default, the root of the input
filesystem is mounted.

#### <span class='hlb-type'>option::mount</span> cache(<span class='hlb-type'>string</span> <span class='hlb-variable'>cacheid</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>sharingmode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>cacheid</span>"
	the unique ID to identify the cache.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>sharingmode</span>"
	the sharing mode of the cache, must be one of the following: - shared: can be used concurrently by multiple writers. - private: creates a new mount if there are multiple writers. - locked: pauses additional writers until the first one releases the mount.

Cache a snapshot of the mount after the run command has executed. A cacheid
must be provided to uniquely identify the cache mount.
Compilers and package managers commonly have an option to specify cache
directories. Depending on their implementation, it may be safe to share the
cache with concurrent processes. This is adjusted via the &#x60;sharingmode&#x60;
argument.
The cache is modified every time the parent run command is executed. A cache
could also be managed by not using the &#x60;cache&#x60; option. Instead, the mount can
be aliased, and then pushed as an image, so that there it can be a stable
snapshot, or updated externally.


#### <span class='hlb-type'>option::run</span> network(<span class='hlb-type'>string</span> <span class='hlb-variable'>networkmode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>networkmode</span>"
	the network mode of the container, must be one of the following: - unset: use the default network provider. - host: use the host&#x27;s network namespace. - none: disable networking.

Sets the networking mode for the duration of the run command. By default, the
value is &quot;unset&quot; (using BuildKit&#x27;s CNI provider, otherwise its host
namespace).



#### <span class='hlb-type'>option::run</span> readonlyRootfs()


Sets the rootfs as read-only for the duration of the run command.



#### <span class='hlb-type'>option::run</span> secret(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	the filepath for a secure file or directory.
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	the directory where the secret is attached.

Mounts a secure file for the duration of the run command. Secrets are
attached via a tmpfs mount, so all the data stays in volatile memory.


#### <span class='hlb-type'>option::secret</span> uid(<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>"
	the user id.

Sets the user ID for the secure file. By default, the UID is 0.

#### <span class='hlb-type'>option::secret</span> gid(<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>"
	the group id.

Sets the group ID for the secure file. By default, the GID is 0.

#### <span class='hlb-type'>option::secret</span> mode(<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>"
	the new permissions of the secure file in octal.

Sets the permissions for the secure file. By default, the file mode is 0600.


#### <span class='hlb-type'>option::run</span> security(<span class='hlb-type'>string</span> <span class='hlb-variable'>securitymode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>securitymode</span>"
	the security mode of the container, must be one of the following: - sandbox: use the default containerd seccomp profile. - insecure: enables all capabilities.

Sets the security mode for the duration of the run command. By default, the
value is &quot;sandbox&quot;.



#### <span class='hlb-type'>option::run</span> ssh()


Mounts a SSH socket for the duration of the run command. By default, it will
try to use the SSH socket found from $SSH_AUTH_SOCK. Otherwise, an option
&#x60;localPath&#x60; can be provided to specify a filepath to a SSH auth socket or
*.pem file.


#### <span class='hlb-type'>option::ssh</span> target(<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	the directory where the SSH agent socket is attached.

Sets the target directory to mount the SSH agent socket. By default, it is
mounted to &#x60;/run/buildkit/ssh_agent.${N}&#x60;, where N is the index of the SSH
socket. If $SSH_AUTH_SOCK is not set, it will set SSH_AUTH_SOCK to the
mountPoint.

#### <span class='hlb-type'>option::ssh</span> localPath(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	the path to a local SSH agent socket or PEM key.

Sets the path to a local SSH agent socket or PEM key, with support for
passphrases. By default, the SSH agent defined by $SSH_AUTH_SOCK will be
mounted into the container.

#### <span class='hlb-type'>option::ssh</span> uid(<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>"
	the user ID.

Sets the user ID for the SSH agent socket. By default, the UID is 0.

#### <span class='hlb-type'>option::ssh</span> gid(<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>id</span>"
	the group ID.

Sets the group ID for the SSH agent socket. By default, the GID is 0.

#### <span class='hlb-type'>option::ssh</span> mode(<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>octal</span> <span class='hlb-variable'>filemode</span>"
	the new permissions of the SSH agent socket in octal.

Sets the permissions for the SSH agent socket. By default, the file mode is
0600.


#### <span class='hlb-type'>option::run</span> user(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the user.

Sets the current user for the duration of the run command.




### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) shell(<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>"
	the list of args used to prefix &#x60;run&#x60; statements.

Sets the current shell command to use when executing subsequent &#x60;run&#x60;
methods. By default, this is [&quot;sh&quot;, &quot;-c&quot;].

	#!hlb
	fs default() {
		scratch
		shell "arg"
	}


### <span class='hlb-type'>fs</span> (<span class='hlb-type'>fs</span>) user(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	the name of the user.

Sets the current user for all subsequent calls in this filesystem block.

	#!hlb
	fs default() {
		scratch
		user "name"
	}



<style>
.hlb-type {
	color: #d73a49
}

.hlb-variable {
	color: #0366d6
}
</style>
