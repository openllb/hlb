package llbutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
)

// LocalID returns a consistent hash for this local (path + options) so that
// the same content doesn't get transported multiple times when referenced
// repeatedly.
func LocalID(ctx context.Context, absPath string, opts ...llb.LocalOption) (string, error) {
	uniqID, err := LocalUniqueID(absPath)
	if err != nil {
		return "", err
	}
	opts = append(opts, llb.LocalUniqueID(uniqID))
	st := llb.Local("", opts...)

	def, err := st.Marshal(ctx)
	if err != nil {
		return "", err
	}

	// The terminal op of the graph def.Def[len(def.Def)-1] is an empty vertex with
	// an input to the last vertex's digest. Since that vertex also has its digests
	// of its inputs and so on, the digest of the terminal op is a merkle hash for
	// the graph.
	return digest.FromBytes(def.Def[len(def.Def)-1]).String(), nil
}

// LocalUniqueID returns a consistent string that is unique per host + cwd
func LocalUniqueID(cwd string) (string, error) {
	mac, err := FirstUpInterface()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("cwd:%s,mac:%s", cwd, mac), nil
}

// FirstUpInterface returns the mac address for the first "UP" network
// interface.
func FirstUpInterface() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // not up
		}
		if iface.HardwareAddr.String() == "" {
			continue // no mac
		}
		return iface.HardwareAddr.String(), nil

	}
	return "no-valid-interface", nil
}

func SecretID(path string) string {
	return digest.FromString(path).String()
}

func SSHID(paths ...string) string {
	return digest.FromString(strings.Join(paths, "")).String()
}

func OutputFromWriter(w io.WriteCloser) func(map[string]string) (io.WriteCloser, error) {
	return func(map[string]string) (io.WriteCloser, error) {
		return w, nil
	}
}
