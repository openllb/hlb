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
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
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

	td := NewTextDocument(uri, params.TextDocument.Text)

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

	return ls.server.Push(ctx, "textDocument/semanticHighlighting", params)
}

func highlightModule(lines map[int]lsp.SemanticHighlightingTokens, mod *parser.Module) {
	parser.Inspect(mod, func(node parser.Node) bool {
		if node == nil {
			return false
		}

		switch n := node.(type) {
		case *parser.Comment:
			highlightNode(lines, node, Comment)
			return false
		case *parser.ImportDecl:
			if n.Import != nil {
				highlightNode(lines, n.Import, Keyword)
			}
			if n.Ident != nil {
				highlightNode(lines, n.Ident, Module)
			}
			switch {
			case n.ImportFunc != nil:
				if n.ImportFunc.From != nil {
					highlightNode(lines, n.ImportFunc.From, Keyword)
				}
				if n.ImportFunc.Func != nil {
					lit := n.ImportFunc.Func
					if lit.Type != nil {
						highlightNode(lines, lit.Type, Type)
					}
					if lit.Type != nil && lit.Body != nil {
						highlightBlock(lines, lit.Type.ObjType, lit.Body)
					}
				}
			case n.ImportPath != nil:
				highlightNode(lines, n.ImportPath, String)
			}
			return false
		case *parser.ExportDecl:
			if n.Export != nil {
				highlightNode(lines, n.Export, Keyword)
			}
			if n.Ident != nil {
				highlightNode(lines, n.Ident, Variable)
			}
			return false
		case *parser.FuncDecl:
			if n.Type != nil {
				highlightNode(lines, n.Type, Type)
			}
			if n.Name != nil {
				highlightNode(lines, n.Name, Function)
			}
			if n.Params != nil {
				for _, field := range n.Params.List {
					if field.Variadic != nil {
						highlightNode(lines, field.Variadic, Modifier)
					}
					if field.Type != nil {
						highlightNode(lines, field.Type, Type)
					}
					if field.Name != nil {
						highlightNode(lines, field.Name, Parameter)
					}
				}
			}
			if n.Type != nil && n.Body != nil {
				highlightBlock(lines, n.Type.ObjType, n.Body)
			}
			return false
		}

		return true
	})
}

func highlightBlock(lines map[int]lsp.SemanticHighlightingTokens, typ parser.ObjType, block *parser.BlockStmt) {
	parser.Inspect(block, func(node parser.Node) bool {
		if node == nil {
			return false
		}

		switch n := node.(type) {
		case *parser.Comment:
			highlightNode(lines, node, Comment)
		case *parser.CallStmt:
			var ident *parser.Ident
			switch {
			case n.Func.Ident != nil:
				ident = n.Func.Ident
				lookupByType, ok := builtin.Lookup.ByType[typ]
				if ok {
					_, ok = lookupByType.Func[ident.Name]
					if !ok {
						highlightNode(lines, ident, Variable)
					}
				}
			case n.Func.Selector != nil:
				ident = n.Func.Selector.Ident
				if ident != nil {
					highlightNode(lines, ident, Module)
				}
				if n.Func.Selector.Select != nil {
					highlightNode(lines, n.Func.Selector.Select, Variable)
				}
			default:
				return true
			}

			for _, arg := range n.Args {
				switch {
				case arg.Bad != nil:
				case arg.Selector != nil:
					if arg.Selector.Ident != nil {
						highlightNode(lines, arg.Selector.Ident, Module)
					}
					if arg.Selector.Select != nil {
						highlightNode(lines, arg.Selector.Select, Variable)
					}
				case arg.Ident != nil:
					highlightNode(lines, arg.Ident, Variable)
				case arg.BasicLit != nil:
					switch {
					case arg.BasicLit.Str != nil:
						highlightNode(lines, arg.BasicLit, String)
					case arg.BasicLit.Decimal != nil, arg.BasicLit.Numeric != nil:
						highlightNode(lines, arg.BasicLit, Numeric)
					case arg.BasicLit.Bool != nil:
						highlightNode(lines, arg.BasicLit, Constant)
					}
				case arg.FuncLit != nil:
					if arg.FuncLit.Type != nil {
						highlightNode(lines, arg.FuncLit.Type, Type)
					}

					if arg.FuncLit.Type != nil && arg.FuncLit.Body != nil {
						highlightBlock(lines, arg.FuncLit.Type.ObjType, arg.FuncLit.Body)
					}
				}
			}

			if n.WithOpt != nil {
				if n.WithOpt.With != nil {
					highlightNode(lines, n.WithOpt.With, Keyword)
				}

				switch {
				case n.WithOpt.Ident != nil:
				case n.WithOpt.FuncLit != nil:
					lit := n.WithOpt.FuncLit
					highlightNode(lines, lit.Type, Type)

					if lit.Type.Primary() == parser.Option {
						typ := parser.ObjType(fmt.Sprintf("%s::%s", lit.Type.Primary(), ident))
						highlightBlock(lines, typ, lit.Body)
					} else {
						highlightBlock(lines, lit.Type.ObjType, lit.Body)
					}
				}
			}

			if n.Alias != nil {
				if n.Alias.As != nil {
					highlightNode(lines, n.Alias.As, Keyword)
				}
				if n.Alias.Ident != nil {
					highlightNode(lines, n.Alias.Ident, Function)
				}
			}

			return false
		}
		return true
	})
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
			td := NewTextDocument(uri, change.Text)
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

	var loc *lsp.Location

	pos := params.Position

	parser.Inspect(td.Module, func(node parser.Node) bool {
		if node == nil || !isPositionWithinNode(pos, node) {
			return false
		}

		switch n := node.(type) {
		case *parser.ExportDecl:
			if isPositionWithinNode(pos, n.Ident) {
				loc = newLocationFromIdent(td.Module.Scope, uri, n.Ident.Name)
			}
		case *parser.FuncDecl:
			fun := n
			parser.Inspect(fun, func(node parser.Node) bool {
				if node == nil || !isPositionWithinNode(pos, node) {
					return false
				}

				switch n := node.(type) {
				case *parser.Expr:
					switch {
					case n.Ident != nil, n.Selector != nil:
						var name string
						switch {
						case n.Ident != nil:
							name = n.Ident.Name
						case n.Selector != nil:
							if isPositionWithinNode(pos, n.Selector.Ident) {
								name = n.Selector.Ident.Name
							} else if isPositionWithinNode(pos, n.Selector.Select) {
								obj := fun.Scope.Lookup(n.Selector.Ident.Name)
								if obj == nil {
									return false
								}

								decl, ok := obj.Node.(*parser.ImportDecl)
								if !ok {
									return false
								}

								rootDir := filepath.Dir(strings.TrimPrefix(string(uri), "file://"))

								var filename string

								switch {
								case decl.ImportFunc != nil:
									cg, err := codegen.New()
									if err != nil {
										log.Printf("failed to create codegen: %s", err)
										return false
									}

									st, err := cg.GenerateImport(ctx, td.Module.Scope, decl.ImportFunc.Func)
									if err != nil {
										log.Printf("failed to generate import: %s", err)
										return false
									}

									def, err := st.Marshal(ctx, llb.LinuxAmd64)
									if err != nil {
										log.Printf("failed to marshal import vertex: %s", err)
										return false
									}

									dgst := digest.FromBytes(def.Def[len(def.Def)-1])
									vp := module.VendorPath(filepath.Join(rootDir, module.ModulesPath), dgst)
									filename = filepath.Join(vp, module.ModuleFilename)
								case decl.ImportPath != nil:
									filename = filepath.Join(rootDir, decl.ImportPath.Path)
								}

								importUri := lsp.DocumentURI(fmt.Sprintf("file://%s", filename))

								ls.tmu.Lock()
								importTD, ok := ls.tds[importUri]
								if !ok {
									data, err := ioutil.ReadFile(filename)
									if err != nil {
										log.Printf("failed to read file: %s", err)
										return false
									}

									importTD = NewTextDocument(importUri, string(data))
									ls.tds[importUri] = importTD
								}
								ls.tmu.Unlock()

								loc = newLocationFromIdent(importTD.Module.Scope, importUri, n.Selector.Select.Name)
								return false
							}
						}

						loc = newLocationFromIdent(fun.Scope, uri, name)
						return false
					case n.FuncLit != nil:
						return true
					default:
						return false
					}
				}
				return true
			})
			return false
		}
		return true
	})

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
	switch obj.Kind {
	case parser.DeclKind:
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			loc = newLocationFromNode(uri, n.Name)
		case *parser.AliasDecl:
			loc = newLocationFromNode(uri, n.Ident)
		case *parser.ImportDecl:
			loc = newLocationFromNode(uri, n.Ident)
		default:
			log.Printf("%s unknown decl kind", checker.FormatPos(n.Position()))
		}
	case parser.FieldKind, parser.ExprKind:
		switch n := obj.Node.(type) {
		case *parser.Field:
			loc = newLocationFromNode(uri, n.Name)
		default:
			log.Printf("%s unknown decl kind", checker.FormatPos(n.Position()))
		}
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

	var (
		h   lsp.Hover
		typ parser.ObjType
	)

	parser.Inspect(td.Module, func(node parser.Node) bool {
		if node == nil || !isPositionWithinNode(pos, node) {
			return false
		}

		switch n := node.(type) {
		case *parser.FuncDecl:
			if n.Type != nil {
				typ = n.Type.ObjType
			}
		case *parser.FuncLit:
			if n.Type != nil {
				typ = n.Type.ObjType
			}
		case *parser.Ident:
			r := newRangeFromNode(node)
			h.Range = &r

			lookupByType, ok := builtin.Lookup.ByType[typ]
			if !ok {
				return false
			}

			_, ok = lookupByType.Func[n.Name]
			if !ok {
				return false
			}

			h.Contents = []lsp.MarkedString{
				{
					Language: "hlb",
					Value:    n.Name,
				},
			}
		}
		return true
	})
	return &h, nil
}

func (ls *LangServer) textDocumentCompletionHandler(ctx context.Context, params lsp.CompletionParams) (*lsp.CompletionList, error) {
	return nil, nil
}

func isPositionWithinNode(pos lsp.Position, node parser.Node) bool {
	if (pos.Line < node.Position().Line-1 || pos.Line > node.End().Line-1) ||
		(pos.Line == node.Position().Line-1 && pos.Character < node.Position().Column-1) ||
		(pos.Line == node.End().Line-1 && pos.Character >= node.End().Column-1) {
		return false
	}

	return true
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

func NewTextDocument(uri lsp.DocumentURI, text string) TextDocument {
	td := TextDocument{
		Identifier: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{
				URI: uri,
			},
		},
		Text: text,
	}

	td.Module, _, td.Err = hlb.Parse(strings.NewReader(text))
	if td.Err != nil {
		log.Printf("failed to parse hlb: %s", td.Err)
		return td
	}
	td.Module.Pos.Filename = strings.TrimPrefix(string(uri), "file://")

	td.Err = checker.Check(td.Module)
	if td.Err != nil {
		log.Printf("failed to check hlb: %s", td.Err)
	}
	return td
}
