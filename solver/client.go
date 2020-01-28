package solver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/appdefaults"
)

// BuildkitClient returns a basic buildkit client.
func BuildkitClient(ctx context.Context) (*client.Client, error) {
	opts := []client.ClientOpt{client.WithFailFast()}
	return client.New(ctx, appdefaults.Address, opts...)
}

var (
	// CloudBuildDiscoveryApp is the current membership definition for our cloudbuild app
	CloudBuildDiscoveryApp = "http://discoveryreadonly.us-west-2.dynprod.netflix.net:7001/v2/apps/cloudbuild"

	// CloudBuildVIP is the vipAddress string that shrimpi registers with:
	CloudBuildVIP = "cloudbuild-stable.netflix.net:7001"

	// CloudBuildSAN is the TLS server name to use for metatron.
	CloudBuildSAN = "cloudbuild.us-west-2.prod.stub.metatron.netflix.net"
)

func MetatronClient(ctx context.Context) (*client.Client, error) {
	opts := []client.ClientOpt{client.WithFailFast()}

	var (
		caCert string
		cert   string
		key    string
	)

	if os.Getenv("CI") != "" {
		caCert = "/metatron/certificates/metatronClient.trust.pem"
		cert = "/run/metatron/certificates/client.crt"
		key = "/run/metatron/certificates/client.key"
	} else {
		srcMetatronDir := filepath.Join(os.Getenv("HOME"), ".metatron")
		caCert = filepath.Join(srcMetatronDir, "metatronClient.trust.pem")
		cert = filepath.Join(srcMetatronDir, "user.crt")
		key = filepath.Join(srcMetatronDir, "user.key")
	}

	opts = append(opts, client.WithCredentials(CloudBuildSAN, caCert, cert, key))

	ip, err := discoveryAddr()
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("tcp://%s:8980", ip)

	return client.New(ctx, uri, opts...)
}

func discoveryAddr() (string, error) {
	discoveryApp := struct {
		Application struct {
			Instances []struct {
				Status     string `json:"status"`
				IPAddr     string `json:"ipAddr"`
				VIPAddress string `json:"vipAddress"`
			} `json:"instance"`
		} `json:"application"`
	}{}

	req, err := http.NewRequest("GET", CloudBuildDiscoveryApp, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(body, &discoveryApp)
	if err != nil {
		return "", err
	}

	for _, instance := range discoveryApp.Application.Instances {
		if instance.VIPAddress == CloudBuildVIP && instance.Status == "UP" {
			return instance.IPAddr, nil
		}
	}

	return "", fmt.Errorf("failed to find available lightning build instance")
}
