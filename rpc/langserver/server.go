package langserver

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser"
	lsp "github.com/sourcegraph/go-lsp"
)

type LangServer struct {
	server *jrpc2.Server
	capset map[Capability]struct{}

	tds map[lsp.DocumentURI]TextDocument
	tmu sync.RWMutex

	dbs map[lsp.DocumentURI]*debouncer
	dmu sync.Mutex
}

type Capability int

const (
	_ Capability = iota
	SemanticHighlightingCapability
)

func NewServer() *LangServer {
	ls := &LangServer{
		capset: make(map[Capability]struct{}),
		tds:    make(map[lsp.DocumentURI]TextDocument),
		dbs:    make(map[lsp.DocumentURI]*debouncer),
	}

	ls.server = jrpc2.NewServer(handler.Map{
		"initialize":              handler.New(ls.initializeHandler),
		"exit":                    handler.New(ls.exitHandler),
		"$/cancelRequest":         handler.New(ls.cancelRequestHandler),
		"textDocument/didOpen":    handler.New(ls.textDocumentDidOpenHandler),
		"textDocument/didClose":   handler.New(ls.textDocumentDidCloseHandler),
		"textDocument/didChange":  handler.New(ls.textDocumentDidChangeHandler),
		"textDocument/hover":      handler.New(ls.textDocumentHoverHandler),
		"textDocument/definition": handler.New(ls.textDocumentDefinitionHandler),
		"textDocument/completion": handler.New(ls.textDocumentCompletionHandler),
	}, &jrpc2.ServerOptions{
		AllowPush: true,
	})

	return ls
}

func (ls *LangServer) Listen(ctx context.Context, r io.Reader, w io.WriteCloser) error {
	defer func() {
		r := recover()
		if r != nil {
			log.Printf("listen recovered panic: %s", r)
		}
	}()

	log.Printf("hlb-langserver listening")
	s := ls.server.Start(channel.Header("")(r, w))
	return s.Wait()
}

func (ls *LangServer) initializeHandler(ctx context.Context, params lsp.InitializeParams) (lsp.InitializeResult, error) {
	log.Printf("initialize %q", params.RootURI)

	highlightCap := params.Capabilities.TextDocument.SemanticHighlightingCapabilities
	if highlightCap != nil && highlightCap.SemanticHighlighting {
		ls.capset[SemanticHighlightingCapability] = struct{}{}
		log.Printf("detected cap semantic highlighting")
	}

	return lsp.InitializeResult{
		Capabilities: lsp.ServerCapabilities{
			DefinitionProvider: true,
			HoverProvider:      true,
			TextDocumentSync: &lsp.TextDocumentSyncOptionsOrKind{
				Options: &lsp.TextDocumentSyncOptions{
					OpenClose: true,
					Change:    lsp.TDSKFull,
				},
			},
			SemanticHighlighting: &lsp.SemanticHighlightingOptions{
				Scopes: [][]string{
					{String.String()},
					{Constant.String()},
					{Numeric.String()},
					{Variable.String()},
					{Parameter.String()},
					{Keyword.String()},
					{Modifier.String()},
					{Type.String()},
					{Function.String()},
					{Module.String()},
					{Comment.String()},
				},
			},
		},
	}, nil
}

func (ls *LangServer) exitHandler(ctx context.Context, params lsp.None) error {
	log.Printf("exit")
	return nil
}

func (ls *LangServer) cancelRequestHandler(ctx context.Context, params lsp.None) error {
	log.Printf("cancel request")
	return nil
}

func (ls *LangServer) textDocumentDidOpenHandler(ctx context.Context, params lsp.DidOpenTextDocumentParams) error {
	uri := params.TextDocument.URI
	log.Printf("did open %q", uri)

	td := NewTextDocument(ctx, uri, params.TextDocument.Text)

	ls.tmu.Lock()
	ls.tds[uri] = td
	ls.tmu.Unlock()

	if _, ok := ls.capset[SemanticHighlightingCapability]; ok {
		go func() {
			err := ls.publishSemanticHighlighting(ctx, td)
			if err != nil {
				log.Printf("err: %s", err)
			}
		}()
	}

	return nil
}

func (ls *LangServer) publishSemanticHighlighting(ctx context.Context, td TextDocument) error {
	log.Printf("publishing semantic highlighting")
	params := lsp.SemanticHighlightingParams{
		TextDocument: td.Identifier,
	}

	lines := make(map[int]lsp.SemanticHighlightingTokens)
	highlightModule(lines, td.Module)

	var sortedLines []int
	for line := range lines {
		sortedLines = append(sortedLines, line)
	}
	sort.Ints(sortedLines)

	for _, line := range sortedLines {
		params.Lines = append(params.Lines, lsp.SemanticHighlightingInformation{
			Line:   line,
			Tokens: lines[line],
		})
	}

	return ls.server.Notify(ctx, "textDocument/semanticHighlighting", params)
}

func highlightModule(lines map[int]lsp.SemanticHighlightingTokens, mod *parser.Module) {
	parser.Match(mod, parser.MatchOpts{},
		func(comment *parser.Comment) {
			highlightNode(lines, comment, Comment)
		},
		func(id *parser.ImportDecl) {
			if id.Import != nil {
				highlightNode(lines, id.Import, Keyword)
			}
			if id.Name != nil {
				highlightNode(lines, id.Name, Module)
			}
			if id.DeprecatedPath != nil {
				if id.DeprecatedPath.Start != nil {
					highlightNode(lines, id.DeprecatedPath.Start, String)
				}
				for _, f := range id.DeprecatedPath.Fragments {
					highlightStringFragment(lines, f)
				}
				if id.DeprecatedPath.Terminate != nil {
					highlightNode(lines, id.DeprecatedPath.Terminate, String)
				}
			}
			if id.From != nil {
				highlightNode(lines, id.From, Keyword)
			}
			if id.Expr != nil {
				highlightExpr(lines, id.Expr)
			}
		},
		func(ed *parser.ExportDecl) {
			if ed.Export != nil {
				highlightNode(lines, ed.Export, Keyword)
			}
			if ed.Name != nil {
				highlightNode(lines, ed.Name, Variable)
			}
		},
		func(fun *parser.FuncDecl) {
			if fun.Type != nil {
				highlightNode(lines, fun.Type, Type)
			}
			if fun.Name != nil {
				highlightNode(lines, fun.Name, Function)
			}
			if fun.Params != nil {
				for _, field := range fun.Params.Fields() {
					if field.Modifier != nil {
						if field.Modifier.Variadic != nil {
							highlightNode(lines, field.Modifier.Variadic, Modifier)
						}
					}
					if field.Type != nil {
						highlightNode(lines, field.Type, Type)
					}
					if field.Name != nil {
						highlightNode(lines, field.Name, Parameter)
					}
				}
			}
			if fun.Effects != nil {
				if fun.Effects.Binds != nil {
					highlightNode(lines, fun.Effects.Binds, Keyword)
				}
				for _, field := range fun.Effects.Effects.Fields() {
					if field.Type != nil {
						highlightNode(lines, field.Type, Type)
					}
					if field.Name != nil {
						highlightNode(lines, field.Name, Parameter)
					}
				}
			}
			if fun.Body != nil {
				highlightBlock(lines, fun.Body)
			}
		},
	)
}

func highlightBlock(lines map[int]lsp.SemanticHighlightingTokens, block *parser.BlockStmt) {
	parser.Match(block, parser.MatchOpts{},
		func(call *parser.CallStmt) {
			if call.Name != nil {
				highlightIdentExpr(lines, call.Name)
			}

			for _, arg := range call.Args {
				highlightExpr(lines, arg)
			}

			if call.WithClause != nil {
				if call.WithClause.With != nil {
					highlightNode(lines, call.WithClause.With, Keyword)
				}

				if call.WithClause.Expr != nil {
					highlightExpr(lines, call.WithClause.Expr)
				}
			}

			if call.BindClause != nil {
				if call.BindClause.As != nil {
					highlightNode(lines, call.BindClause.As, Keyword)
				}
				if call.BindClause.Ident != nil {
					highlightNode(lines, call.BindClause.Ident, Function)
				}
				if call.BindClause.Binds != nil {
					for _, b := range call.BindClause.Binds.Binds() {
						highlightNode(lines, b.Source, Parameter)
						highlightNode(lines, b.Target, Function)
					}
				}
			}

		},
		func(expr *parser.ExprStmt) {
			if expr.Expr != nil {
				highlightExpr(lines, expr.Expr)
			}
		},
	)
}

func highlightExpr(lines map[int]lsp.SemanticHighlightingTokens, expr *parser.Expr) {
	switch {
	case expr.FuncLit != nil:
		if expr.FuncLit.Type != nil {
			highlightNode(lines, expr.FuncLit.Type, Type)
		}

		if expr.FuncLit.Body != nil {
			highlightBlock(lines, expr.FuncLit.Body)
		}
	case expr.BasicLit != nil:
		switch {
		case expr.BasicLit.Decimal != nil:
			highlightNode(lines, expr.BasicLit, Numeric)
		case expr.BasicLit.Numeric != nil:
			highlightNode(lines, expr.BasicLit.Numeric, Numeric)
		case expr.BasicLit.Bool != nil:
			highlightNode(lines, expr.BasicLit, Constant)
		case expr.BasicLit.Str != nil:
			if expr.BasicLit.Str.Start != nil {
				highlightNode(lines, expr.BasicLit.Str.Start, String)
			}
			for _, f := range expr.BasicLit.Str.Fragments {
				highlightStringFragment(lines, f)
			}
			if expr.BasicLit.Str.Terminate != nil {
				highlightNode(lines, expr.BasicLit.Str.Terminate, String)
			}
		case expr.BasicLit.RawString != nil:
			highlightNode(lines, expr.BasicLit.RawString, String)
		case expr.BasicLit.Heredoc != nil:
			for _, f := range expr.BasicLit.Heredoc.Fragments {
				highlightHeredocFragment(lines, f)
			}
		case expr.BasicLit.RawHeredoc != nil:
			for _, f := range expr.BasicLit.RawHeredoc.Fragments {
				highlightHeredocFragment(lines, f)
			}
		}
	case expr.CallExpr != nil:
		call := expr.CallExpr
		if call.Name != nil {
			highlightIdentExpr(lines, call.Name)
		}
		for _, arg := range call.Args() {
			highlightExpr(lines, arg)
		}
	}
}

func highlightStringFragment(lines map[int]lsp.SemanticHighlightingTokens, f *parser.StringFragment) {
	switch {
	case f.Escaped != nil:
		highlightNode(lines, f, Comment)
	case f.Interpolated != nil:
		highlightExpr(lines, f.Interpolated.Expr)
	case f.Text != nil:
		highlightNode(lines, f, String)
	}
}

func highlightHeredocFragment(lines map[int]lsp.SemanticHighlightingTokens, f *parser.HeredocFragment) {
	switch {
	case f.Escaped != nil:
		highlightNode(lines, f, Comment)
	case f.Interpolated != nil:
		highlightExpr(lines, f.Interpolated.Expr)
	}
}

func highlightIdentExpr(lines map[int]lsp.SemanticHighlightingTokens, ie *parser.IdentExpr) {
	if ie.Reference != nil {
		if ie.Ident != nil {
			highlightNode(lines, ie.Ident, Module)
		}
		highlightNode(lines, ie.Reference, Variable)
	} else if ie.Ident != nil {
		highlightNode(lines, ie.Ident, Variable)
	}
}

func highlightNode(lines map[int]lsp.SemanticHighlightingTokens, node parser.Node, s Scope) {
	line := node.Position().Line - 1
	lines[line] = append(lines[line], lsp.SemanticHighlightingToken{
		Character: uint32(node.Position().Column - 1),
		Length:    uint16(node.End().Column - node.Position().Column),
		Scope:     uint16(s),
	})
}

func (ls *LangServer) textDocumentDidCloseHandler(ctx context.Context, params lsp.DidCloseTextDocumentParams) error {
	log.Printf("text document did close %#v", params)
	return nil
}

func (ls *LangServer) textDocumentDidChangeHandler(ctx context.Context, params lsp.DidChangeTextDocumentParams) error {
	uri := params.TextDocument.URI
	log.Printf("did change %q", uri)

	return ls.debounce(uri, 10*time.Millisecond, func() error {
		ls.tmu.Lock()
		defer ls.tmu.Unlock()

		_, ok := ls.tds[uri]
		if !ok {
			return fmt.Errorf("unknown uri %q", uri)
		}

		for _, change := range params.ContentChanges {
			td := NewTextDocument(ctx, uri, change.Text)
			if _, ok := ls.capset[SemanticHighlightingCapability]; ok {
				go func() {
					err := ls.publishSemanticHighlighting(ctx, td)
					if err != nil {
						log.Printf("err: %s", err)
					}
				}()
			}

			ls.tds[uri] = td
		}
		return nil
	})
}

type debouncer struct {
	timer        *time.Timer
	mu           sync.Mutex
	publish      chan func() error
	subscription chan error
}

func newDebouncer(interval time.Duration) *debouncer {
	d := &debouncer{
		timer:   time.NewTimer(interval),
		publish: make(chan func() error),
	}

	go func() {
		var f func() error
		for {
			select {
			case f = <-d.publish:
				d.timer.Reset(interval)
			case <-d.timer.C:
				d.mu.Lock()
				d.subscription <- f()
				d.subscription = nil
				d.mu.Unlock()
			}
		}
	}()

	return d
}

func (d *debouncer) debounce(subscription chan error, f func() error) {
	d.mu.Lock()
	if d.subscription != nil {
		d.subscription <- nil
	}
	d.publish <- f
	d.subscription = subscription
	d.mu.Unlock()
}

func (ls *LangServer) debounce(uri lsp.DocumentURI, interval time.Duration, f func() error) error {
	ls.dmu.Lock()
	debouncer, ok := ls.dbs[uri]
	if !ok {
		debouncer = newDebouncer(interval)
		ls.dbs[uri] = debouncer
	}
	ls.dmu.Unlock()

	subscription := make(chan error)
	debouncer.debounce(subscription, f)

	return <-subscription
}

func (ls *LangServer) textDocumentDefinitionHandler(ctx context.Context, params lsp.TextDocumentPositionParams) ([]lsp.Location, error) {
	defer func() {
		r := recover()
		if r != nil {
			log.Printf("panic: %q", r)
		}
	}()

	uri := params.TextDocument.URI
	log.Printf("text document definition %q", uri)

	ls.tmu.RLock()
	td, ok := ls.tds[uri]
	if !ok {
		ls.tmu.RUnlock()
		return nil, fmt.Errorf("unknown uri %q", uri)
	}
	ls.tmu.RUnlock()

	var (
		loc *lsp.Location
		pos = params.Position
	)

	parser.Match(td.Module,
		parser.MatchOpts{
			Filter: func(node parser.Node) bool {
				return isPositionWithinNode(pos, node)
			},
		},
		func(_ *parser.ExportDecl, ident *parser.Ident) {
			loc = newLocationFromIdent(td.Module.Scope, uri, ident.Text)
		},
		func(block *parser.BlockStmt, ie *parser.IdentExpr) {
			if isPositionWithinNode(pos, ie.Ident) {
				loc = newLocationFromIdent(block.Scope, uri, ie.Ident.Text)
				return
			} else if !isPositionWithinNode(pos, ie.Reference) {
				return
			}

			obj := block.Scope.Lookup(ie.Ident.Text)
			if obj == nil {
				return
			}

			id, ok := obj.Node.(*parser.ImportDecl)
			if !ok {
				return
			}

			cg, err := codegen.New(nil)
			if err != nil {
				log.Printf("failed to create codegen: %s", err)
				return
			}

			ret := codegen.NewRegister(ctx)
			err = cg.EmitExpr(ctx, block.Scope, id.Expr, nil, nil, nil, ret)
			if err != nil {
				log.Printf("failed to generate import: %s", err)
				return
			}

			rootDir := filepath.Dir(strings.TrimPrefix(string(uri), "file://"))
			var filename string
			switch ret.Kind() {
			case parser.Filesystem:
				fs, err := ret.Filesystem()
				if err != nil {
					return
				}

				def, err := fs.State.Marshal(ctx, llb.LinuxAmd64)
				if err != nil {
					log.Printf("failed to marshal import vertex: %s", err)
					return
				}

				dgst := digest.FromBytes(def.Def[len(def.Def)-1])
				vp := module.VendorPath(filepath.Join(rootDir, module.ModulesPath), dgst)
				filename = filepath.Join(vp, module.ModuleFilename)
			case parser.String:
				localPath, err := ret.String()
				if err != nil {
					return
				}

				filename = filepath.Join(rootDir, localPath)
			default:
				return
			}

			importUri := lsp.DocumentURI(fmt.Sprintf("file://%s", filename))

			ls.tmu.Lock()
			importTD, ok := ls.tds[importUri]
			if !ok {
				data, err := ioutil.ReadFile(filename)
				if err != nil {
					log.Printf("failed to read file: %s", err)
					ls.tmu.Unlock()
					return
				}

				importTD = NewTextDocument(ctx, importUri, string(data))
				ls.tds[importUri] = importTD
			}
			ls.tmu.Unlock()

			loc = newLocationFromIdent(importTD.Module.Scope, importUri, ie.Reference.Ident.Text)
		},
	)

	var locs []lsp.Location
	if loc != nil {
		locs = append(locs, *loc)
	}

	return locs, nil
}

func newLocationFromIdent(scope *parser.Scope, uri lsp.DocumentURI, name string) *lsp.Location {
	obj := scope.Lookup(name)
	if obj == nil {
		return nil
	}

	var loc *lsp.Location
	switch n := obj.Node.(type) {
	case *parser.FuncDecl:
		loc = newLocationFromNode(uri, n.Name)
	case *parser.BindClause:
		if n.Ident != nil {
			loc = newLocationFromNode(uri, n.Ident)
		}
		if n.Binds != nil {
			for _, b := range n.Binds.Binds() {
				if b.Target.String() == name {
					loc = newLocationFromNode(uri, b.Target)
					break
				}
			}
			if loc == nil {
				loc = newLocationFromNode(uri, n)
			}
		}
	case *parser.ImportDecl:
		loc = newLocationFromNode(uri, n.Name)
	case *parser.Field:
		loc = newLocationFromNode(uri, n.Name)
	default:
		log.Printf("%s unknown decl kind", parser.FormatPos(n.Position()))
	}

	return loc
}

func (ls *LangServer) textDocumentHoverHandler(ctx context.Context, params lsp.TextDocumentPositionParams) (*lsp.Hover, error) {
	ls.tmu.Lock()
	uri := params.TextDocument.URI
	td, ok := ls.tds[uri]
	if !ok {
		ls.tmu.Unlock()
		return nil, fmt.Errorf("unknown uri %q", uri)
	}
	ls.tmu.Unlock()

	pos := params.Position

	var h lsp.Hover

	parser.Match(td.Module,
		parser.MatchOpts{
			AllowDuplicates: true,
			Filter: func(node parser.Node) bool {
				return isPositionWithinNode(pos, node)
			},
		},
		func(block *parser.BlockStmt, ident *parser.Ident) {
			lookupByKind, ok := builtin.Lookup.ByKind[block.Kind()]
			if !ok {
				return
			}

			fun, ok := lookupByKind.Func[ident.Text]
			if !ok {
				return
			}

			paramsBlock := ""
			if len(fun.Params) > 0 {
				var params []string
				for _, param := range fun.Params {
					params = append(params, fmt.Sprintf("%s %s", param.Type, param.Name))
				}

				paramsBlock = fmt.Sprintf("(%s)", strings.Join(params, ", "))
			}

			effectsBlock := ""
			if len(fun.Effects) > 0 {
				var effects []string
				for _, effect := range fun.Effects {
					effects = append(effects, fmt.Sprintf("%s %s", effect.Type, effect.Name))
				}

				effectsBlock = fmt.Sprintf(" as (%s)", strings.Join(effects, ", "))
			}

			r := newRangeFromNode(ident)
			h.Range = &r
			h.Contents = []lsp.MarkedString{
				{
					Language: "hlb",
					Value:    fmt.Sprintf("%s%s%s", ident, paramsBlock, effectsBlock),
				},
			}
		},
	)
	return &h, nil
}

func (ls *LangServer) textDocumentCompletionHandler(ctx context.Context, params lsp.CompletionParams) (*lsp.CompletionList, error) {
	return nil, nil
}

func isPositionWithinNode(pos lsp.Position, node parser.Node) bool {
	return parser.IsPositionWithinNode(node, pos.Line-1, pos.Character-1)
}

func newLocationFromNode(uri lsp.DocumentURI, node parser.Node) *lsp.Location {
	return &lsp.Location{
		URI:   uri,
		Range: newRangeFromNode(node),
	}
}

func newRangeFromNode(node parser.Node) lsp.Range {
	return lsp.Range{
		Start: lsp.Position{Line: node.Position().Line - 1, Character: node.Position().Column - 1},
		End:   lsp.Position{Line: node.End().Line - 1, Character: node.End().Column - 1},
	}
}

type TextDocument struct {
	Identifier lsp.VersionedTextDocumentIdentifier
	Module     *parser.Module
	Text       string
	Err        error
}

func NewTextDocument(ctx context.Context, uri lsp.DocumentURI, text string) TextDocument {
	td := TextDocument{
		Identifier: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{
				URI: uri,
			},
		},
		Text: text,
	}

	td.Module, td.Err = parser.Parse(ctx, strings.NewReader(text))
	if td.Err != nil {
		log.Printf("failed to parse hlb: %s", td.Err)
		return td
	}
	td.Module.Pos.Filename = strings.TrimPrefix(string(uri), "file://")

	td.Err = checker.SemanticPass(td.Module)
	if td.Err != nil {
		log.Printf("failed to semantic pass hlb: %s", td.Err)
		return td
	}

	_ = linter.Lint(ctx, td.Module)

	td.Err = checker.Check(td.Module)
	if td.Err != nil {
		log.Printf("failed to check hlb: %s", td.Err)
	}
	return td
}
