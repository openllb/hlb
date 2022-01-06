## <span class='hlb-type'>fs</span> functions
### <span class='hlb-type'>fs</span> <span class='hlb-name'>breakpoint</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>"
	



	#!hlb
	fs default() {
		breakpoint "command"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>cmd</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>"
	



	#!hlb
	fs default() {
		cmd "args"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>copy</span>(<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>dst</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>dst</span>"
	



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
			excludePatterns "pattern"
			followSymlinks
			includePatterns "pattern"
			unpack
		}
	}


#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>allowEmptyWildcard</span>()




#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>allowWildcard</span>()




#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>chmod</span>(<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	



#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>chown</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	



#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>contentsOnly</span>()




#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>createDestPath</span>()




#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>createdTime</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	



#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>excludePatterns</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	



#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>followSymlinks</span>()




#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>includePatterns</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	



#### <span class='hlb-type'>option::copy</span> <span class='hlb-name'>unpack</span>()





### <span class='hlb-type'>fs</span> <span class='hlb-name'>diff</span>(<span class='hlb-type'>fs</span> <span class='hlb-variable'>base</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>base</span>"
	



	#!hlb
	fs default() {
		diff scratch
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>dir</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	



	#!hlb
	fs default() {
		dir "path"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>dockerLoad</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	



	#!hlb
	fs default() {
		dockerLoad "ref"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>dockerPush</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	



	#!hlb
	fs default() {
		dockerPush "ref"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>download</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	



	#!hlb
	fs default() {
		download "localPath"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>downloadDockerTarball</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	



	#!hlb
	fs default() {
		downloadDockerTarball "localPath" "ref"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>downloadOCITarball</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	



	#!hlb
	fs default() {
		downloadOCITarball "localPath"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>downloadTarball</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	



	#!hlb
	fs default() {
		downloadTarball "localPath"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>entrypoint</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>"
	



	#!hlb
	fs default() {
		entrypoint "args"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>env</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	



	#!hlb
	fs default() {
		env "key" "value"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>expose</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ports</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ports</span>"
	



	#!hlb
	fs default() {
		expose "ports"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>frontend</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>source</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>source</span>"
	



	#!hlb
	fs default() {
		frontend "source" with option {
			input "key" scratch
			opt "key" "value"
		}
	}


#### <span class='hlb-type'>option::frontend</span> <span class='hlb-name'>input</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>fs</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	
!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>value</span>"
	



#### <span class='hlb-type'>option::frontend</span> <span class='hlb-name'>opt</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	




### <span class='hlb-type'>fs</span> <span class='hlb-name'>git</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>remote</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>remote</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	



	#!hlb
	fs default() {
		git "remote" "ref" with option {
			keepGitDir
		}
	}


#### <span class='hlb-type'>option::git</span> <span class='hlb-name'>keepGitDir</span>()





### <span class='hlb-type'>fs</span> <span class='hlb-name'>http</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>url</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>url</span>"
	



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
	



#### <span class='hlb-type'>option::http</span> <span class='hlb-name'>chmod</span>(<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	



#### <span class='hlb-type'>option::http</span> <span class='hlb-name'>filename</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	




### <span class='hlb-type'>fs</span> <span class='hlb-name'>image</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	



	#!hlb
	fs default() {
		image "ref" with option {
			platform "os" "arch"
			resolve
		}
	}


#### <span class='hlb-type'>option::image</span> <span class='hlb-name'>platform</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>os</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>arch</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>os</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arch</span>"
	



#### <span class='hlb-type'>option::image</span> <span class='hlb-name'>resolve</span>()





### <span class='hlb-type'>fs</span> <span class='hlb-name'>label</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	



	#!hlb
	fs default() {
		label "key" "value"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>local</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	



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
	



#### <span class='hlb-type'>option::local</span> <span class='hlb-name'>followPaths</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	



#### <span class='hlb-type'>option::local</span> <span class='hlb-name'>includePatterns</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>pattern</span>"
	




### <span class='hlb-type'>fs</span> <span class='hlb-name'>merge</span>(<span class='hlb-type'>fs</span> <span class='hlb-variable'>inputs</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>inputs</span>"
	



	#!hlb
	fs default() {
		merge scratch
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>mkdir</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>, <span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	
!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	



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
	



#### <span class='hlb-type'>option::mkdir</span> <span class='hlb-name'>createParents</span>()




#### <span class='hlb-type'>option::mkdir</span> <span class='hlb-name'>createdTime</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	




### <span class='hlb-type'>fs</span> <span class='hlb-name'>mkfile</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>, <span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>content</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	
!!! info "<span class='hlb-type'>int</span> <span class='hlb-variable'>filemode</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>content</span>"
	



	#!hlb
	fs default() {
		mkfile "path" 0 "content" with option {
			chown "owner"
			createdTime "created"
		}
	}


#### <span class='hlb-type'>option::mkfile</span> <span class='hlb-name'>chown</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>owner</span>"
	



#### <span class='hlb-type'>option::mkfile</span> <span class='hlb-name'>createdTime</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>created</span>"
	




### <span class='hlb-type'>fs</span> <span class='hlb-name'>rm</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	



	#!hlb
	fs default() {
		rm "path" with option {
			allowNotFound
			allowWildcard
		}
	}


#### <span class='hlb-type'>option::rm</span> <span class='hlb-name'>allowNotFound</span>()




#### <span class='hlb-type'>option::rm</span> <span class='hlb-name'>allowWildcard</span>()





### <span class='hlb-type'>fs</span> <span class='hlb-name'>run</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>"
	



	#!hlb
	fs default() {
		run "arg" with option {
			breakpoint "command"
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


#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>breakpoint</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>dir</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>path</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>env</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>forward</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>dest</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>src</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>dest</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>host</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>hostname</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>address</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>hostname</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>address</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>ignoreCache</span>()




#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>mount</span>(<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>fs</span> <span class='hlb-variable'>input</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>network</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>networkmode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>networkmode</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>readonlyRootfs</span>()




#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>secret</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>localPath</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountPoint</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>security</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>securitymode</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>securitymode</span>"
	



#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>shlex</span>()




#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>ssh</span>()




#### <span class='hlb-type'>option::run</span> <span class='hlb-name'>user</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	




### <span class='hlb-type'>fs</span> <span class='hlb-name'>scratch</span>()




	#!hlb
	fs default() {
		scratch
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>shell</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arg</span>"
	



	#!hlb
	fs default() {
		shell "arg"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>stopSignal</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>signal</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>signal</span>"
	



	#!hlb
	fs default() {
		stopSignal "signal"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>user</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	



	#!hlb
	fs default() {
		user "name"
	}



### <span class='hlb-type'>fs</span> <span class='hlb-name'>volumes</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>mountpoints</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>mountpoints</span>"
	



	#!hlb
	fs default() {
		volumes "mountpoints"
	}



## <span class='hlb-type'>string</span> functions
### <span class='hlb-type'>string</span> <span class='hlb-name'>format</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>formatString</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>values</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>formatString</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>values</span>"
	



	#!hlb
	string myString() {
		format "formatString" "values"
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localArch</span>()




	#!hlb
	string myString() {
		localArch
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localCwd</span>()




	#!hlb
	string myString() {
		localCwd
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localEnv</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>key</span>"
	



	#!hlb
	string myString() {
		localEnv "key"
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localOs</span>()




	#!hlb
	string myString() {
		localOs
	}



### <span class='hlb-type'>string</span> <span class='hlb-name'>localRun</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>command</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>args</span>"
	



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




#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>includeStderr</span>()




#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>onlyStderr</span>()




#### <span class='hlb-type'>option::localRun</span> <span class='hlb-name'>shlex</span>()





### <span class='hlb-type'>string</span> <span class='hlb-name'>manifest</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>ref</span>"
	



	#!hlb
	string myString() {
		manifest "ref" with option {
			platform "os" "arch"
		}
	}


#### <span class='hlb-type'>option::manifest</span> <span class='hlb-name'>platform</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>os</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>arch</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>os</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>arch</span>"
	




### <span class='hlb-type'>string</span> <span class='hlb-name'>template</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>text</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>text</span>"
	



	#!hlb
	string myString() {
		template "text" with option {
			stringField "name" "value"
		}
	}


#### <span class='hlb-type'>option::template</span> <span class='hlb-name'>stringField</span>(<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>, <span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>)

!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>name</span>"
	
!!! info "<span class='hlb-type'>string</span> <span class='hlb-variable'>value</span>"
	





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
