package filebuffer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/moby/buildkit/client/llb"
)

type buffersKey struct{}

func WithBuffers(ctx context.Context, buffers *BufferLookup) context.Context {
	return context.WithValue(ctx, buffersKey{}, buffers)
}

func Buffers(ctx context.Context) *BufferLookup {
	buffers, ok := ctx.Value(buffersKey{}).(*BufferLookup)
	if !ok {
		return NewBuffers()
	}
	return buffers
}

type BufferLookup struct {
	fbs map[string]*FileBuffer
	mu  sync.Mutex
}

func NewBuffers() *BufferLookup {
	return &BufferLookup{
		fbs: make(map[string]*FileBuffer),
	}
}

func (b *BufferLookup) Get(filename string) *FileBuffer {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.fbs[filename]
}

func (b *BufferLookup) Set(filename string, fb *FileBuffer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fbs[filename] = fb
}

func (b *BufferLookup) All() []*FileBuffer {
	var filenames []string
	for filename := range b.fbs {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)
	var fbs []*FileBuffer
	for _, filename := range filenames {
		fbs = append(fbs, b.Get(filename))
	}
	return fbs
}

type FileBuffer struct {
	filename  string
	buf       bytes.Buffer
	offset    int
	offsets   []int
	mu        sync.Mutex
	onDisk    bool
	sourceMap *llb.SourceMap
}

type Option func(*FileBuffer)

func WithEphemeral() Option {
	return func(fb *FileBuffer) {
		fb.onDisk = false
	}
}

func New(filename string, opts ...Option) *FileBuffer {
	fb := &FileBuffer{
		filename: filename,
		onDisk:   true,
	}
	for _, opt := range opts {
		opt(fb)
	}
	return fb
}

func (fb *FileBuffer) Filename() string {
	return fb.filename
}

func (fb *FileBuffer) OnDisk() bool {
	return fb.onDisk
}

func (fb *FileBuffer) SourceMap() *llb.SourceMap {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.sourceMap == nil {
		// This caching is important - BuildKit dedups SourceMaps based
		// on pointer address, so returning a fresh SourceMap each time
		// would blow up the size of the solve request.
		fb.sourceMap = llb.NewSourceMap(nil, fb.filename, fb.buf.Bytes())
	}
	return fb.sourceMap
}

func (fb *FileBuffer) Len() int {
	return len(fb.offsets)
}

func (fb *FileBuffer) Bytes() []byte {
	return fb.buf.Bytes()
}

func (fb *FileBuffer) Write(p []byte) (n int, err error) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	fb.sourceMap = nil

	n, err = fb.buf.Write(p)

	start := 0
	index := bytes.IndexByte(p[:n], byte('\n'))
	for index >= 0 {
		fb.offsets = append(fb.offsets, fb.offset+start+index)
		start += index + 1
		index = bytes.IndexByte(p[start:n], byte('\n'))
	}
	fb.offset += n

	return n, err
}

func (fb *FileBuffer) Position(line, column int) lexer.Position {
	var offset int
	if line-2 < 0 {
		offset = column - 1
	} else {
		offset = fb.offsets[line-2] + column - 1
	}
	return lexer.Position{
		Filename: fb.filename,
		Offset:   offset,
		Line:     line,
		Column:   column,
	}
}

func (fb *FileBuffer) Segment(offset int) ([]byte, error) {
	if len(fb.offsets) == 0 {
		return fb.buf.Bytes(), nil
	}

	index := fb.findNearestLineIndex(offset)
	start := 0
	if index >= 0 {
		start = fb.offsets[index] + 1
	}

	if start > fb.buf.Len()-1 {
		return nil, io.EOF
	}

	var end int
	if offset < fb.offsets[len(fb.offsets)-1] {
		end = fb.offsets[index+1]
	} else {
		end = fb.buf.Len()
	}

	return fb.read(start, end)
}

func (fb *FileBuffer) Line(ln int) ([]byte, error) {
	if ln > len(fb.offsets) {
		return nil, fmt.Errorf("line %d outside of offsets", ln)
	}

	start := 0
	if ln > 0 {
		start = fb.offsets[ln-1] + 1
	}

	end := fb.offsets[0]
	if ln > 0 {
		end = fb.offsets[ln]
	}

	return fb.read(start, end)
}

func (fb *FileBuffer) findNearestLineIndex(offset int) int {
	index := sort.Search(len(fb.offsets), func(i int) bool {
		return fb.offsets[i] >= offset
	})

	if index < len(fb.offsets) {
		if fb.offsets[index] < offset {
			return index
		}
		return index - 1
	} else {
		// If offset is further than any newline, then the last newline is the
		// nearest.
		return index - 1
	}
}

func (fb *FileBuffer) read(start, end int) ([]byte, error) {
	r := bytes.NewReader(fb.buf.Bytes())

	_, err := r.Seek(int64(start), io.SeekStart)
	if err != nil {
		return nil, err
	}

	line := make([]byte, end-start)
	n, err := r.Read(line)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return line[:n], nil
}
