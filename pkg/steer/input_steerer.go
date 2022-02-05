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
// into the appropriate pipe depending whether we have an exec session
// active.
type InputSteerer struct {
	mu  sync.Mutex
	pws []*io.PipeWriter
}

func NewInputSteerer(inputReader io.Reader, pws ...*io.PipeWriter) *InputSteerer {
	is := &InputSteerer{
		pws: pws,
	}

	go func() {
		var p [4096]byte
		for {
			n, err := inputReader.Read(p[:])
			var pw *io.PipeWriter
			is.mu.Lock()
			if len(is.pws) != 0 {
				pw = is.pws[len(is.pws)-1]
			}
			is.mu.Unlock()
			if n != 0 && pw != nil {
				pw.Write(p[:n])
			}
			if err != nil {
				is.mu.Lock()
				defer is.mu.Unlock()
				for _, pw := range is.pws {
					pw.CloseWithError(err)
				}
				return
			}
		}
	}()
	return is
}

// Push pushes a new pipe to steer input to, until Pop is called to steer it
// back to the previous pipe.
func (is *InputSteerer) Push(pw *io.PipeWriter) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.pws = append(is.pws, pw)
}

// Pop causes future input to be directed to the pipe where it was going before
// the last call to Push.
func (is *InputSteerer) Pop() {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.pws = is.pws[:len(is.pws)-1]
}
