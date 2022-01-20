package codegen

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	"github.com/xlab/treeprint"
)

var (
	PrototypeIn  []reflect.Type
	PrototypeOut []reflect.Type
)

type Prototype struct{}

func (p Prototype) Call(ctx context.Context, cln *client.Client, ret Register, opts Option) error {
	return nil
}

type Option []interface{}

type Register interface {
	Value
	Set(interface{}) error
}

type register struct {
	Value
	ctor func(iface interface{}) (Value, error)
}

func NewRegister(ctx context.Context) Register {
	return &register{
		Value: ZeroValue(ctx),
		ctor: func(iface interface{}) (Value, error) {
			return NewValue(ctx, iface)
		},
	}
}

func (r *register) Set(iface interface{}) (err error) {
	r.Value, err = r.ctor(iface)
	return err
}

type Value interface {
	Kind() parser.Kind
	Filesystem() (Filesystem, error)
	String() (string, error)
	Int() (int, error)
	Option() (Option, error)
	Request() (solver.Request, error)
	Reflect(reflect.Type) (reflect.Value, error)
}

func NewValue(ctx context.Context, iface interface{}) (Value, error) {
	switch v := iface.(type) {
	case Value:
		return v, nil
	case Filesystem:
		return &fsValue{&nilValue{}, v}, validateState(v.State)
	case llb.State:
		zero := ZeroValue(ctx)
		fs, err := zero.Filesystem()
		if err != nil {
			return zero, err
		}
		fs.State = v
		return &fsValue{&nilValue{}, fs}, validateState(v)
	case string:
		return &stringValue{&nilValue{}, v}, nil
	case int:
		return &intValue{&nilValue{}, v}, nil
	case Option:
		return &optValue{&nilValue{}, v}, nil
	case solver.Request:
		return &reqValue{&nilValue{}, v}, nil
	default:
		return nil, fmt.Errorf("invalid value type %#v", iface)
	}
}

func validateState(st llb.State) error {
	ctx := context.Background()
	c := &llb.Constraints{}
	if st.Output() != nil && st.Output().Vertex(ctx, c) != nil {
		return st.Validate(ctx, c)
	}
	return nil
}

type nilValue struct{}

func (v *nilValue) Kind() parser.Kind {
	return parser.None
}

func (v *nilValue) Filesystem() (Filesystem, error) {
	return Filesystem{}, fmt.Errorf("cannot coerce to fs")
}

func (v *nilValue) Int() (int, error) {
	return 0, fmt.Errorf("cannot coerce to int")
}

func (v *nilValue) String() (string, error) {
	return "", fmt.Errorf("cannot coerce to string")
}

func (v *nilValue) Option() (Option, error) {
	return nil, fmt.Errorf("cannot coerce to option")
}

func (v *nilValue) Request() (solver.Request, error) {
	return solver.NilRequest(), nil
}

func (v *nilValue) Reflect(t reflect.Type) (reflect.Value, error) {
	return reflect.Value{}, fmt.Errorf("cannot reflect nil value")
}

type zeroValue struct {
	defaultPlatform specs.Platform
}

func ZeroValue(ctx context.Context) Value {
	return &zeroValue{
		defaultPlatform: DefaultPlatform(ctx),
	}
}

func (v *zeroValue) Kind() parser.Kind {
	return parser.None
}

func (v *zeroValue) Filesystem() (Filesystem, error) {
	return Filesystem{
		State:    llb.Scratch(),
		Image:    &solver.ImageSpec{},
		Platform: v.defaultPlatform,
	}, nil
}

func (v *zeroValue) Int() (int, error) {
	return 0, nil
}

func (v *zeroValue) String() (string, error) {
	return "", nil
}

func (v *zeroValue) Option() (Option, error) {
	return Option([]interface{}{}), nil
}

func (v *zeroValue) Request() (solver.Request, error) {
	return solver.NilRequest(), nil
}

func (v *zeroValue) Reflect(t reflect.Type) (reflect.Value, error) {
	return ReflectTo(v, t)
}

type Filesystem struct {
	State       llb.State
	Image       *solver.ImageSpec
	SolveOpts   []solver.SolveOption
	SessionOpts []llbutil.SessionOption
	Platform    specs.Platform
}

func (fs Filesystem) Digest(ctx context.Context) (digest.Digest, error) {
	c := &llb.Constraints{}
	dgst, _, _, _, err := fs.State.Output().Vertex(ctx, c).Marshal(ctx, &llb.Constraints{})
	return dgst, err
}

func (fs Filesystem) Tree() (treeprint.Tree, error) {
	def, err := fs.State.Marshal(context.Background())
	if err != nil {
		return nil, err
	}

	tree := treeprint.New()
	return tree, solver.TreeFromDef(tree, def, fs.SolveOpts)
}

type fsValue struct {
	Value
	fs Filesystem
}

func (v *fsValue) Kind() parser.Kind {
	return parser.Filesystem
}

func (v *fsValue) Filesystem() (Filesystem, error) {
	var image solver.ImageSpec
	if v.fs.Image != nil {
		image = *v.fs.Image
	}
	fs := Filesystem{
		State:       v.fs.State,
		Image:       &image,
		SolveOpts:   make([]solver.SolveOption, len(v.fs.SolveOpts)),
		SessionOpts: make([]llbutil.SessionOption, len(v.fs.SessionOpts)),
		Platform:    v.fs.Platform,
	}
	copy(fs.SolveOpts, v.fs.SolveOpts)
	copy(fs.SessionOpts, v.fs.SessionOpts)
	return fs, nil
}

func (v *fsValue) Request() (solver.Request, error) {
	def, err := v.fs.State.Marshal(context.Background(), llb.Platform(v.fs.Platform))
	if err != nil {
		return nil, err
	}

	return solver.Single(&solver.Params{
		Def:         def,
		SolveOpts:   v.fs.SolveOpts,
		SessionOpts: v.fs.SessionOpts,
	}), nil
}

func (v *fsValue) Reflect(t reflect.Type) (reflect.Value, error) {
	return ReflectTo(v, t)
}

type stringValue struct {
	Value
	str string
}

func (v *stringValue) Kind() parser.Kind {
	return parser.String
}

func (v *stringValue) String() (string, error) {
	return v.str, nil
}

func (v *stringValue) Int() (int, error) {
	return strconv.Atoi(v.str)
}

func (v *stringValue) Bool() (bool, error) {
	return strconv.ParseBool(v.str)
}

func (v *stringValue) Reflect(t reflect.Type) (reflect.Value, error) {
	return ReflectTo(v, t)
}

type intValue struct {
	Value
	i int
}

func (v *intValue) Kind() parser.Kind {
	return parser.Int
}

func (v *intValue) Int() (int, error) {
	return v.i, nil
}

func (v *intValue) String() (string, error) {
	return strconv.Itoa(v.i), nil
}

func (v *intValue) Reflect(t reflect.Type) (reflect.Value, error) {
	return ReflectTo(v, t)
}

type optValue struct {
	Value
	opt Option
}

func (v *optValue) Kind() parser.Kind {
	return parser.Option
}

func (v *optValue) Option() (Option, error) {
	return v.opt, nil
}

type reqValue struct {
	Value
	req solver.Request
}

func (v *reqValue) Kind() parser.Kind {
	return parser.Pipeline
}

func (v *reqValue) Request() (solver.Request, error) {
	return v.req, nil
}

func (v *reqValue) Reflect(t reflect.Type) (reflect.Value, error) {
	return ReflectTo(v, t)
}

var (
	rFilesystem = reflect.TypeOf(Filesystem{})
	rString     = reflect.TypeOf("")
	rInt        = reflect.TypeOf(0)
	rOption     = reflect.TypeOf((Option)([]interface{}{}))
	rRequest    = reflect.TypeOf((*solver.Request)(nil)).Elem()
	rFileMode   = reflect.TypeOf(os.FileMode(0))
	rDigest     = reflect.TypeOf(digest.Digest(""))
	rTime       = reflect.TypeOf(time.Time{})
	rIP         = reflect.TypeOf(net.IP(nil))
	rURL        = reflect.TypeOf(&url.URL{})
)

func ReflectTo(v Value, t reflect.Type) (reflect.Value, error) {
	var (
		iface interface{}
		err   error
	)

	switch t {
	case rFilesystem:
		iface, err = v.Filesystem()
	case rString:
		iface, err = v.String()
	case rInt:
		iface, err = v.Int()
	case rOption:
		iface, err = v.Option()
	case rRequest:
		iface, err = v.Request()
	case rFileMode:
		var i int
		i, err = v.Int()
		if err != nil {
			return reflect.Value{}, err
		}

		iface = os.FileMode(i)
	case rDigest:
		var str string
		str, err = v.String()
		if err != nil {
			return reflect.Value{}, err
		}

		iface, err = digest.Parse(str)
	case rTime:
		var str string
		str, err = v.String()
		if err != nil {
			return reflect.Value{}, err
		}

		iface, err = time.Parse(time.RFC3339, str)
	case rIP:
		var str string
		str, err = v.String()
		if err != nil {
			return reflect.Value{}, err
		}

		ip := net.ParseIP(str)
		if ip == nil {
			return reflect.Value{}, fmt.Errorf("invalid ip %q", str)
		}
		iface = ip
	case rURL:
		var str string
		str, err = v.String()
		if err != nil {
			return reflect.Value{}, err
		}

		iface, err = url.Parse(str)
	default:
		return reflect.Value{}, fmt.Errorf("unrecognized type %s", t)
	}

	return reflect.ValueOf(iface), err
}
