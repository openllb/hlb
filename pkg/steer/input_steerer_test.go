package steer

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fixedReader struct {
	io.Reader
	byteClock chan struct{}
}

func (fr *fixedReader) Read(p []byte) (n int, err error) {
	b := make([]byte, 1)
	n, err = fr.Reader.Read(b)
	copy(p, b)
	<-fr.byteClock
	return
}

func TestInputSteerer(t *testing.T) {
	r := &fixedReader{
		Reader:    strings.NewReader("abc"),
		byteClock: make(chan (struct{}), 1),
	}

	pr, pw := io.Pipe()
	is := NewInputSteerer(r, pw)

	p := make([]byte, 1)
	r.byteClock <- struct{}{}
	n, err := pr.Read(p)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "a", string(p[:n]))

	pr2, pw2 := io.Pipe()
	is.Push(pw2)

	// A new pipe writer was pushed, so reading from the previous pipe reader
	// should block until it is popped off.
	done := make(chan struct{})
	go func() {
		defer close(done)
		p = make([]byte, 1)
		r.byteClock <- struct{}{}
		n, err = pr.Read(p)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}()

	p2 := make([]byte, 1)
	r.byteClock <- struct{}{}
	n, err = pr2.Read(p2)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "b", string(p2[:n]))

	// After popping, the value read should be after what the popped off pipe
	// reader read.
	is.Pop()
	<-done
	require.Equal(t, "c", string(p[:n]))
}
