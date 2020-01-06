package solver

import (
	"context"
	"os"
	"path/filepath"

	"github.com/moby/buildkit/client"
)

// BuildkitClient returns a new buildkit client using metatron TLS.
func BuildkitClient(ctx context.Context) (*client.Client, error) {
	opts := []client.ClientOpt{client.WithFailFast()}

	srcMetatronDir := filepath.Join(os.Getenv("HOME"), ".metatron")
	caCert := filepath.Join(srcMetatronDir, "metatronClient.trust.pem")
	cert := filepath.Join(srcMetatronDir, "user.crt")
	key := filepath.Join(srcMetatronDir, "user.key")

	// CloudBuildSAN is the TLS server name to use for metatron.
	CloudBuildSAN := "cloudbuild.us-west-2.prod.stub.metatron.netflix.net"

	opts = append(opts, client.WithCredentials(CloudBuildSAN, caCert, cert, key))

	uri := "tcp://100.70.131.235:8980"
	return client.New(ctx, uri, opts...)
}
