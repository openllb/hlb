package codegen

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/openllb/hlb/parser"
)

var (
	Callables = map[parser.Kind]map[string]interface{}{
		parser.Filesystem: {
			"scratch":               Scratch{},
			"image":                 Image{},
			"http":                  HTTP{},
			"git":                   Git{},
			"local":                 Local{},
			"frontend":              Frontend{},
			"run":                   Run{},
			"env":                   Env{},
			"dir":                   Dir{},
			"user":                  User{},
			"mkdir":                 Mkdir{},
			"mkfile":                Mkfile{},
			"rm":                    Rm{},
			"copy":                  Copy{},
			"merge":                 Merge{},
			"diff":                  Diff{},
			"entrypoint":            Entrypoint{},
			"cmd":                   Cmd{},
			"label":                 Label{},
			"expose":                Expose{},
			"volumes":               Volumes{},
			"stopSignal":            StopSignal{},
			"dockerPush":            DockerPush{},
			"dockerLoad":            DockerLoad{},
			"download":              Download{},
			"downloadTarball":       DownloadTarball{},
			"downloadOCITarball":    DownloadOCITarball{},
			"downloadDockerTarball": DownloadDockerTarball{},
			"breakpoint":            SetBreakpoint{},
		},
		parser.String: {
			"format":    Format{},
			"template":  Template{},
			"manifest":  Manifest{},
			"localArch": LocalArch{},
			"localOs":   LocalOS{},
			"localCwd":  LocalCwd{},
			"localEnv":  LocalEnv{},
			"localRun":  LocalRun{},
		},
		parser.Pipeline: {
			"stage":    Stage{},
			"parallel": Stage{},
		},
		"option::image": {
			"resolve":  Resolve{},
			"platform": Platform{},
		},
		"option::http": {
			"checksum": Checksum{},
			"chmod":    Chmod{},
			"filename": Filename{},
		},
		"option::git": {
			"keepGitDir": KeepGitDir{},
		},
		"option::local": {
			"includePatterns": IncludePatterns{},
			"excludePatterns": ExcludePatterns{},
			"followPaths":     FollowPaths{},
		},
		"option::frontend": {
			"input": FrontendInput{},
			"opt":   FrontendOpt{},
		},
		"option::run": {
			"readonlyRootfs": ReadonlyRootfs{},
			"env":            RunEnv{},
			"dir":            RunDir{},
			"user":           RunUser{},
			"ignoreCache":    IgnoreCache{},
			"network":        Network{},
			"security":       Security{},
			"shlex":          Shlex{},
			"host":           Host{},
			"ssh":            SSH{},
			"forward":        Forward{},
			"secret":         Secret{},
			"mount":          Mount{},
			"breakpoint":     RunBreakpoint{},
		},
		"option::ssh": {
			"target":     MountTarget{},
			"uid":        UID{},
			"gid":        GID{},
			"mode":       UtilChmod{},
			"localPaths": LocalPaths{},
		},
		"option::secret": {
			"uid":             UID{},
			"gid":             GID{},
			"mode":            UtilChmod{},
			"includePatterns": SecretIncludePatterns{},
			"excludePatterns": SecretExcludePatterns{},
		},
		"option::mount": {
			"readonly":   Readonly{},
			"tmpfs":      Tmpfs{},
			"sourcePath": SourcePath{},
			"cache":      Cache{},
		},
		"option::mkdir": {
			"createParents": CreateParents{},
			"chown":         Chown{},
			"createdTime":   CreatedTime{},
		},
		"option::mkfile": {
			"chown":       Chown{},
			"createdTime": CreatedTime{},
		},
		"option::rm": {
			"allowNotFound": AllowNotFound{},
			"allowWildcard": AllowWildcard{},
		},
		"option::copy": {
			"followSymlinks":     FollowSymlinks{},
			"contentsOnly":       ContentsOnly{},
			"unpack":             Unpack{},
			"createDestPath":     CreateDestPath{},
			"allowWildcard":      CopyAllowWildcard{},
			"allowEmptyWildcard": AllowEmptyWildcard{},
			"chown":              UtilChown{},
			"chmod":              UtilChmod{},
			"createdTime":        UtilCreatedTime{},
			"includePatterns":    CopyIncludePatterns{},
			"excludePatterns":    CopyExcludePatterns{},
		},
		"option::localRun": {
			"ignoreError":   IgnoreError{},
			"onlyStderr":    OnlyStderr{},
			"includeStderr": IncludeStderr{},
			"shlex":         Shlex{},
		},
		"option::template": {
			"stringField": StringField{},
		},
		"option::manifest": {
			"platform": Platform{},
		},
	}
)

func init() {
	err := initCallables()
	if err != nil {
		panic(err)
	}
}

func initCallables() error {
	protoCall, ok := reflect.TypeOf(Prototype{}).MethodByName("Call")
	if !ok {
		return fmt.Errorf("Prototype has no Call method")
	}

	// Build prototype signature.
	for i := 1; i < protoCall.Type.NumIn(); i++ {
		PrototypeIn = append(PrototypeIn, protoCall.Type.In(i))
	}
	for i := 0; i < protoCall.Type.NumOut(); i++ {
		PrototypeOut = append(PrototypeOut, protoCall.Type.Out(i))
	}

	// Type check all the builtin functions.
	var errs []string
	for _, byKind := range Callables {
		for _, callable := range byKind {
			err := CheckPrototype(callable)
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "\n"))
	}
	return nil
}

func CheckPrototype(callable interface{}) error {
	c := reflect.ValueOf(callable).MethodByName("Call")

	var (
		ins  []reflect.Type
		outs []reflect.Type
	)
	for i := 0; i < c.Type().NumIn(); i++ {
		ins = append(ins, c.Type().In(i))
	}
	for i := 0; i < c.Type().NumOut(); i++ {
		outs = append(outs, c.Type().Out(i))
	}

	err := fmt.Errorf(
		"expected (%s).Call(%s)(%s) to match Call(%s)(%s)",
		reflect.TypeOf(callable),
		ins,
		outs,
		PrototypeIn,
		PrototypeOut,
	)

	// Verify callable matches prototype signature.
	if c.Type().NumIn() < len(PrototypeIn) || c.Type().NumOut() != len(PrototypeOut) {
		return err
	}
	for i := 0; i < len(PrototypeIn); i++ {
		param := ins[i]
		if (param.Kind() == reflect.Interface && !param.Implements(PrototypeIn[i])) ||
			(param.Kind() != reflect.Interface && param != PrototypeIn[i]) {
			return err
		}
	}
	for i := 0; i < len(PrototypeOut); i++ {
		param := outs[i]
		if (param.Kind() == reflect.Interface && !param.Implements(PrototypeOut[i])) ||
			(param.Kind() != reflect.Interface && param != PrototypeOut[i]) {
			return err
		}
	}

	return nil
}
