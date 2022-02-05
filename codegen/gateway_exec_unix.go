// +build !windows

package codegen

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"golang.org/x/sys/unix"
)

func addResizeHandler(ctx context.Context, proc gateway.ContainerProcess) func() {
	ch := make(chan os.Signal, 1)
	ch <- syscall.SIGWINCH // Initial resize.

	go forwardResize(ctx, ch, proc, int(os.Stdin.Fd()))

	signal.Notify(ch, syscall.SIGWINCH)
	return func() { signal.Stop(ch) }
}

func forwardResize(ctx context.Context, ch chan os.Signal, proc gateway.ContainerProcess, fd int) {
	for {
		select {
		case <-ctx.Done():
			close(ch)
			return
		case <-ch:
			ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
			if err != nil {
				return
			}

			err = proc.Resize(ctx, gateway.WinSize{
				Cols: uint32(ws.Col),
				Rows: uint32(ws.Row),
			})
			if err != nil {
				return
			}
		}
	}
}
