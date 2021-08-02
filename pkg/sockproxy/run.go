package sockproxy

import (
	"io"
	"net"
)

func Run(l net.Listener, dialer func() (net.Conn, error)) error {
	for {
		proxy, err := l.Accept()
		if err != nil {
			return err
		}
		conn, err := dialer()
		if err != nil {
			return err
		}

		go func() {
			defer proxy.Close()
			_, _ = io.Copy(conn, proxy)
		}()

		go func() {
			defer conn.Close()
			_, _ = io.Copy(proxy, conn)
		}()
	}
}
