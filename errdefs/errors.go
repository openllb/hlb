package errdefs

import (
	"fmt"
	"os"
	"strings"

	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser/ast"
	"github.com/pkg/errors"
)

type ErrAbort struct {
	Err     error
	NumErrs int
}

func (e *ErrAbort) Unwrap() error {
	return e.Err
}

func (e *ErrAbort) Error() string {
	if e.NumErrs == 0 {
		return e.Err.Error()
	}
	errStr := "error"
	if e.NumErrs > 1 {
		errStr = fmt.Sprintf("%d errors", e.NumErrs)
	}
	return fmt.Sprintf("\naborting due to previous %s", errStr)
}

func WithAbort(err error, numErrs int) *ErrAbort {
	return &ErrAbort{Err: err, NumErrs: numErrs}
}

type ErrModule struct {
	Module *ast.Module
	Err    error
}

func (e *ErrModule) Unwrap() error {
	return e.Err
}

func (e *ErrModule) Error() string {
	return e.Err.Error()
}

func WithDeprecated(mod *ast.Module, node ast.Node, format string, a ...interface{}) error {
	return node.WithError(
		&ErrModule{mod, fmt.Errorf(format, a...)},
		node.Spanf(diagnostic.Primary, format, a...),
	)
}

func WithInternalErrorf(node ast.Node, format string, a ...interface{}) error {
	return node.WithError(
		fmt.Errorf(format, a...),
		node.Spanf(diagnostic.Primary, format, a...),
	)
}

func WithInvalidCompileTarget(ident ast.Node) error {
	return ident.WithError(
		fmt.Errorf("invalid compile target %s", ident),
		ident.Spanf(diagnostic.Primary, "cannot compile target"),
	)
}

func WithWrongType(expr ast.Node, expected []ast.Kind, actual ast.Kind, opts ...diagnostic.Option) error {
	opts = append(opts, expr.Spanf(
		diagnostic.Primary,
		"cannot use %s as %s", actual, OneOfKinds(expected),
	))
	return expr.WithError(
		fmt.Errorf("cannot use %s as %s", actual, OneOfKinds(expected)),
		opts...,
	)
}

func WithCallImport(ident ast.Node, decl ast.Node) error {
	return ident.WithError(
		fmt.Errorf("cannot call an imported module"),
		ident.Spanf(diagnostic.Primary, "cannot use import directly"),
		decl.Spanf(diagnostic.Secondary, "use dot notation to call exported functions"),
	)
}

func WithImportPathNotExist(err error, expr ast.Node, filename string) error {
	return expr.WithError(
		err,
		expr.Spanf(diagnostic.Primary, "no such file %q", filename),
	)
}

func WithUndefinedIdent(ident ast.Node, suggested *ast.Object, opts ...diagnostic.Option) error {
	opts = append(opts, ident.Spanf(diagnostic.Primary, "undefined or not in scope"))
	if suggested != nil {
		opts = append(opts, suggested.Ident.Spanf(diagnostic.Secondary, "did you mean `%s`?", suggested.Ident))
	}
	return ident.WithError(
		fmt.Errorf("`%s` is undefined or not in scope", ident),
		opts...,
	)
}

func WithNotImport(ie *ast.IdentExpr, decl ast.Node) error {
	return ie.Reference.WithError(
		fmt.Errorf("cannot use dot notation with non-import"),
		ie.Reference.Spanf(diagnostic.Primary, "`%s` is not an import", ie.Ident),
		decl.Spanf(diagnostic.Secondary, "defined here"),
	)
}

func WithCallUnexported(ref ast.Node, opts ...diagnostic.Option) error {
	opts = append(opts, ref.Spanf(
		diagnostic.Primary,
		"cannot call unexported function",
	))
	return ref.WithError(
		fmt.Errorf("cannot call unexported function `%s`", ref),
		opts...,
	)
}

func WithNumArgs(callee ast.Node, expected, actual int, opts ...diagnostic.Option) error {
	opts = append(opts, callee.Spanf(
		diagnostic.Primary,
		"expected %d args, found %d", expected, actual,
	))
	return callee.WithError(
		fmt.Errorf("`%s` expected %d args, found %d", callee, expected, actual),
		opts...,
	)
}

func WithDuplicates(dups []ast.Node) error {
	if len(dups) == 0 {
		return nil
	}
	var opts []diagnostic.Option
	for i, dup := range dups {
		if i == 0 {
			opts = append(opts, dup.Spanf(diagnostic.Secondary, "defined here"))
		} else {
			opts = append(opts, dup.Spanf(diagnostic.Primary, "duplicate"))
		}
	}
	times := " "
	if len(dups) > 2 {
		times = fmt.Sprintf(" %d times ", len(dups)-1)
	}
	return dups[0].WithError(
		fmt.Errorf("`%s` is redefined%sin this scope", dups[0], times),
		opts...,
	)
}

func WithNoBindTarget(as ast.Node) error {
	return as.WithError(
		fmt.Errorf("cannot bind, has no target"),
		as.Spanf(diagnostic.Primary, "no bind target"),
	)
}

func WithNoBindClosure(as, option ast.Node) error {
	return as.WithError(
		fmt.Errorf("cannot bind, no closure in option blocks"),
		as.Spanf(diagnostic.Primary, "no closure for binding"),
		option.Spanf(diagnostic.Secondary, "option blocks have no closures outside of \"with option {...}\""),
	)
}

func WithNoBindEffects(callee, as ast.Node, opts ...diagnostic.Option) error {
	opts = append(opts, as.Spanf(
		diagnostic.Primary,
		"`%s` has no effects to bind", callee,
	))
	return as.WithError(
		fmt.Errorf("cannot bind, `%s` has no function effects", callee),
		opts...,
	)
}

func WithUndefinedBindTarget(callee, target ast.Node) error {
	return target.WithError(
		fmt.Errorf("cannot bind, `%s` is an undefined effect of `%s`", target, callee),
		target.Spanf(diagnostic.Primary, "undefined bind"),
	)
}

func WithInvalidImageRef(err error, arg ast.Node, ref string) error {
	return arg.WithError(
		errors.Wrapf(err, "failed to parse `%s`", ref),
		arg.Spanf(diagnostic.Primary, "failed to parse `%s`\n%s", ref, err),
	)
}

func WithInvalidNetworkMode(arg ast.Node, mode string, modes []string) error {
	suggestion := diagnostic.Suggestion(mode, modes)
	if suggestion != "" {
		suggestion = fmt.Sprintf("\ndid you mean `%s`?", suggestion)
	}
	return arg.WithError(
		fmt.Errorf("invalid network mode `%s`", mode),
		arg.Spanf(diagnostic.Primary, "invalid network mode `%s`%s", mode, suggestion),
	)
}

func WithInvalidSecurityMode(arg ast.Node, mode string, modes []string) error {
	suggestion := diagnostic.Suggestion(mode, modes)
	if suggestion != "" {
		suggestion = fmt.Sprintf("\ndid you mean `%s`?", suggestion)
	}
	return arg.WithError(
		fmt.Errorf("invalid security mode `%s`", mode),
		arg.Spanf(diagnostic.Primary, "invalid security mode `%s`%s", mode, suggestion),
	)
}

func WithInvalidSharingMode(arg ast.Node, mode string, modes []string) error {
	suggestion := diagnostic.Suggestion(mode, modes)
	if suggestion != "" {
		suggestion = fmt.Sprintf("\ndid you mean `%s`?", suggestion)
	}
	return arg.WithError(
		fmt.Errorf("invalid cache sharing mode `%s`", mode),
		arg.Spanf(diagnostic.Primary, "invalid sharing mode `%s`%s", mode, suggestion),
	)
}

func WithBindCacheMount(as, cache ast.Node) error {
	return as.WithError(
		fmt.Errorf("cannot bind a cache mount"),
		as.Spanf(diagnostic.Primary, "cannot bind a cache mount"),
		cache.Spanf(diagnostic.Secondary, "cache mode enabled here"),
	)
}

func WithDockerEngineUnsupported(decl ast.Node) error {
	err := fmt.Errorf("not supported by buildkit embedded in docker engine, use standalone buildkit")
	if decl == nil {
		return err
	}
	return decl.WithError(
		err,
		decl.Spanf(diagnostic.Primary, "not supported by docker engine"),
	)
}

func OneOfKinds(kinds []ast.Kind) string {
	if len(kinds) == 1 {
		return fmt.Sprintf("type %s", kinds[0])
	}
	return fmt.Sprintf("one of types %s", kinds)
}

func Defined(node ast.Node) diagnostic.Option {
	return node.Spanf(diagnostic.Secondary, "defined here")
}

func Imported(node ast.Node) diagnostic.Option {
	return node.Spanf(diagnostic.Secondary, "imported here")
}

func DefinedMaybeImported(scope *ast.Scope, ie *ast.IdentExpr, decl ast.Node) []diagnostic.Option {
	opts := []diagnostic.Option{Defined(decl)}
	if ie.Reference != nil {
		obj := scope.Lookup(ie.Ident.Text)
		if obj != nil {
			opts = append(opts, Imported(obj.Ident))
		}
	}
	return opts
}

func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) || strings.HasSuffix(err.Error(), "no such file or directory")
}
