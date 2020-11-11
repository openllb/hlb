package sockproxy

import (
	"context"
	"io"
	"net"
)

func Run(ctx context.Context, conn net.Conn, l net.Listener) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		proxy, err := l.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer proxy.Close()
			_, _ = io.Copy(conn, proxy)
		}()

		go func() {
			_, _ = io.Copy(proxy, conn)
		}()
	}
}
