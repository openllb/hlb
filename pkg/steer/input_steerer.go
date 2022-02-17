package steer

import (
	"io"
	"sync"
)

// InputSteerer is a mechanism for directing input to one of a set of
// Readers. This is used when the debugger runs an exec: we can't
// interrupt a Read from the exec context, so if we naively passed the
// primary reader into the exec, it would swallow the next debugger
// command after the exec session ends. To work around this, have a
// goroutine which continuously reads from the input, and steers data
// into the appropriate writer depending whether we have an exec session
// active.
type InputSteerer struct {
	mu sync.Mutex
	ws []io.WriteCloser
}

func NewInputSteerer(inputReader io.Reader, ws ...io.WriteCloser) *InputSteerer {
	is := &InputSteerer{ws: ws}

	go func() {
		var p [4096]byte
		for {
			n, err := inputReader.Read(p[:])
			var w io.WriteCloser
			is.mu.Lock()
			if len(is.ws) != 0 {
				w = is.ws[len(is.ws)-1]
			}
			is.mu.Unlock()
			if n != 0 && w != nil {
				w.Write(p[:n])
			}
			if err != nil {
				is.mu.Lock()
				defer is.mu.Unlock()
				for _, w := range is.ws {
					if pw, ok := w.(*io.PipeWriter); ok {
						pw.CloseWithError(err)
					} else {
						w.Close()
					}
				}
				return
			}
		}
	}()
	return is
}

// Push pushes a new writer to steer input to, until Pop is called to steer it
// back to the previous writer.
func (is *InputSteerer) Push(w io.WriteCloser) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.ws = append(is.ws, w)
}

// Pop causes future input to be directed to the writer where it was going before
// the last call to Push.
func (is *InputSteerer) Pop() {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.ws = is.ws[:len(is.ws)-1]
}
