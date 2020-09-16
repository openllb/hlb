package builtin

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/pkg/errors"
)

var (
	Module *parser.Module

	FileBuffer *filebuffer.FileBuffer

	Callables = map[parser.Kind]map[string]parser.Callable{
		parser.Filesystem: map[string]parser.Callable{
			"scratch":               codegen.Scratch{},
			"image":                 codegen.Image{},
			"http":                  codegen.HTTP{},
			"git":                   codegen.Git{},
			"local":                 codegen.Local{},
			"frontend":              codegen.Frontend{},
			"run":                   codegen.Run{},
			"env":                   codegen.Env{},
			"dir":                   codegen.Dir{},
			"user":                  codegen.User{},
			"mkdir":                 codegen.Mkdir{},
			"mkfile":                codegen.Mkfile{},
			"rm":                    codegen.Rm{},
			"copy":                  codegen.Copy{},
			"entrypoint":            codegen.Entrypoint{},
			"cmd":                   codegen.Cmd{},
			"label":                 codegen.Label{},
			"expose":                codegen.Expose{},
			"volumes":               codegen.Volumes{},
			"stopSignal":            codegen.StopSignal{},
			"dockerPush":            codegen.DockerPush{},
			"dockerLoad":            &codegen.DockerLoad{},
			"download":              codegen.Download{},
			"downloadTarball":       codegen.DownloadTarball{},
			"downloadOCITarball":    codegen.DownloadOCITarball{},
			"downloadDockerTarball": codegen.DownloadDockerTarball{},
		},
		parser.String: map[string]parser.Callable{
			"format":    codegen.Format{},
			"template":  codegen.Template{},
			"manifest":  codegen.Manifest{},
			"localArch": codegen.LocalArch{},
			"localOs":   codegen.LocalOS{},
			"localCwd":  codegen.LocalCwd{},
			"localEnv":  codegen.LocalEnv{},
			"localRun":  codegen.LocalRun{},
		},
		parser.Group: map[string]parser.Callable{
			"parallel": codegen.Stage{},
		},
		"option::image": map[string]parser.Callable{
			"resolve": codegen.Resolve{},
		},
		"option::http": map[string]parser.Callable{
			"checksum": codegen.Checksum{},
			"chmod":    codegen.Chmod{},
			"filename": codegen.Filename{},
		},
		"option::git": map[string]parser.Callable{
			"keepGitDir": codegen.KeepGitDir{},
		},
		"option::local": map[string]parser.Callable{
			"includePatterns": codegen.IncludePatterns{},
			"excludePatterns": codegen.ExcludePatterns{},
			"followPaths":     codegen.FollowPaths{},
		},
		"option::frontend": map[string]parser.Callable{
			"input": codegen.FrontendInput{},
			"opt":   codegen.FrontendOpt{},
		},
		"option::run": map[string]parser.Callable{
			"readonlyRootfs": codegen.ReadonlyRootfs{},
			"env":            codegen.RunEnv{},
			"dir":            codegen.RunDir{},
			"user":           codegen.RunUser{},
			"ignoreCache":    codegen.IgnoreCache{},
			"network":        codegen.Network{},
			"security":       codegen.Security{},
			"shlex":          codegen.Shlex{},
			"host":           codegen.Host{},
			"ssh":            codegen.SSH{},
			"forward":        codegen.Forward{},
			"secret":         codegen.Secret{},
			"mount":          codegen.Mount{},
		},
		"option::ssh": map[string]parser.Callable{
			"target":     codegen.MountTarget{},
			"uid":        codegen.UID{},
			"gid":        codegen.GID{},
			"mode":       codegen.UtilChmod{},
			"localPaths": codegen.LocalPaths{},
		},
		"option::secret": map[string]parser.Callable{
			"uid":             codegen.UID{},
			"gid":             codegen.GID{},
			"mode":            codegen.UtilChmod{},
			"includePatterns": codegen.SecretIncludePatterns{},
			"excludePatterns": codegen.SecretExcludePatterns{},
		},
		"option::mount": map[string]parser.Callable{
			"readonly":   codegen.Readonly{},
			"tmpfs":      codegen.Tmpfs{},
			"sourcePath": codegen.SourcePath{},
			"cache":      codegen.Cache{},
		},
		"option::mkdir": map[string]parser.Callable{
			"createParents": codegen.CreateParents{},
			"chown":         codegen.Chown{},
			"createdTime":   codegen.CreatedTime{},
		},
		"option::mkfile": map[string]parser.Callable{
			"chown":       codegen.Chown{},
			"createdTime": codegen.CreatedTime{},
		},
		"option::rm": map[string]parser.Callable{
			"allowNotFound": codegen.AllowNotFound{},
			"allowWildcard": codegen.AllowWildcard{},
		},
		"option::copy": map[string]parser.Callable{
			"followSymlinks":     codegen.FollowSymlinks{},
			"contentsOnly":       codegen.ContentsOnly{},
			"unpack":             codegen.Unpack{},
			"createDestPath":     codegen.CreateDestPath{},
			"allowWildcard":      codegen.CopyAllowWildcard{},
			"allowEmptyWildcard": codegen.AllowEmptyWildcard{},
			"chown":              codegen.UtilChown{},
			"chmod":              codegen.UtilChmod{},
			"createdTime":        codegen.UtilCreatedTime{},
		},
		"option::localRun": map[string]parser.Callable{
			"ignoreError":   codegen.IgnoreError{},
			"onlyStderr":    codegen.OnlyStderr{},
			"includeStderr": codegen.IncludeStderr{},
			"shlex":         codegen.Shlex{},
		},
		"option::template": map[string]parser.Callable{
			"stringField": codegen.StringField{},
		},
		"option::manifest": map[string]parser.Callable{
			"platform": codegen.Platform{},
		},
	}
)

func init() {
	err := initSources()
	if err != nil {
		panic(err)
	}

	err = initCallables()
	if err != nil {
		panic(err)
	}
}

func initCallables() error {
	protoCall, ok := reflect.TypeOf(codegen.Prototype{}).MethodByName("Call")
	if !ok {
		return fmt.Errorf("Prototype has no Call method")
	}

	// Build prototype signature.
	for i := 1; i < protoCall.Type.NumIn(); i++ {
		codegen.PrototypeIn = append(codegen.PrototypeIn, protoCall.Type.In(i))
	}
	for i := 0; i < protoCall.Type.NumOut(); i++ {
		codegen.PrototypeOut = append(codegen.PrototypeOut, protoCall.Type.Out(i))
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

func initSources() (err error) {
	ctx := diagnostic.WithSources(context.Background(), filebuffer.NewSources())
	Module, err = parser.Parse(ctx, &parser.NamedReader{
		Reader: strings.NewReader(Reference),
		Value:  "<builtin>",
	})
	if err != nil {
		return errors.Wrapf(err, "failed to initialize filebuffer for builtins")
	}
	FileBuffer = diagnostic.Sources(ctx).Get(Module.Pos.Filename)
	return
}

func Sources() *filebuffer.Sources {
	sources := filebuffer.NewSources()
	sources.Set(FileBuffer.Filename(), FileBuffer)
	return sources
}

func CheckPrototype(callable parser.Callable) error {
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
		codegen.PrototypeIn,
		codegen.PrototypeOut,
	)

	// Verify callable matches prototype signature.
	if c.Type().NumIn() < len(codegen.PrototypeIn) || c.Type().NumOut() != len(codegen.PrototypeOut) {
		return err
	}
	for i := 0; i < len(codegen.PrototypeIn); i++ {
		param := ins[i]
		if (param.Kind() == reflect.Interface && !param.Implements(codegen.PrototypeIn[i])) ||
			(param.Kind() != reflect.Interface && param != codegen.PrototypeIn[i]) {
			return err
		}
	}
	for i := 0; i < len(codegen.PrototypeOut); i++ {
		param := outs[i]
		if (param.Kind() == reflect.Interface && !param.Implements(codegen.PrototypeOut[i])) ||
			(param.Kind() != reflect.Interface && param != codegen.PrototypeOut[i]) {
			return err
		}
	}

	return nil
}
