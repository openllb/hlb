package parser

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"github.com/moby/buildkit/client/llb"
)

type FileBuffer struct {
	filename  string
	buf       *bytes.Buffer
	offset    int
	offsets   []int
	sourceMap *llb.SourceMap
}

func NewFileBuffer(filename string) *FileBuffer {
	return &FileBuffer{
		filename: filename,
		buf:      new(bytes.Buffer),
	}
}

func (fb *FileBuffer) SourceMap() *llb.SourceMap {
	if fb.sourceMap == nil {
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

func (fb *FileBuffer) Line(num int) ([]byte, error) {
	if num > len(fb.offsets) {
		return nil, fmt.Errorf("line %d outside of offsets", num)
	}

	start := 0
	if num > 0 {
		start = fb.offsets[num-1] + 1
	}

	end := fb.offsets[0]
	if num > 0 {
		end = fb.offsets[num]
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
