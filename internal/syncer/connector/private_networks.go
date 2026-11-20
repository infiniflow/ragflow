package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

const allowedPrivateNetworksEnv = "RAGFLOW_DATA_SOURCE_ALLOWED_PRIVATE_NETWORKS"

func loadPrivateNetworks() (*common.PrivateNetworks, error) {
	raw := strings.TrimSpace(os.Getenv(allowedPrivateNetworksEnv))
	if raw == "" {
		return nil, nil
	}
	var entries []string
	if err := json.Unmarshal([]byte(raw), &entries); err != nil || entries == nil {
		return nil, fmt.Errorf("%s must be a JSON array of IP addresses or CIDRs", allowedPrivateNetworksEnv)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	networks, err := common.ParsePrivateNetworks(entries)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", allowedPrivateNetworksEnv, err)
	}
	return &networks, nil
}

func resolveConnectorURL(rawURL string) (string, string, error) {
	networks, err := loadPrivateNetworks()
	if err != nil {
		return "", "", err
	}
	if networks == nil {
		return common.AssertURLSafe(rawURL)
	}
	return networks.AssertURLSafe(rawURL, common.LookupHost)
}

func resolveConnectorHost(host string) (string, error) {
	networks, err := loadPrivateNetworks()
	if err != nil {
		return "", err
	}
	if networks != nil {
		return networks.AssertHostSafe(host)
	}
	if pinned, enabled, err := common.PinConfiguredPrivateHost(host); enabled {
		return pinned, err
	}
	return connectorAssertHostSafe(host)
}

// fetchConnectorRemoteFile gives connector downloads the same network policy
// as API requests while preserving the shared downloader's size limits.
func fetchConnectorRemoteFile(ctx context.Context, rawURL string, maxSize int64, timeout time.Duration) ([]byte, http.Header, string, error) {
	networks, err := loadPrivateNetworks()
	if err != nil {
		return nil, nil, "", err
	}
	if networks == nil {
		return utility.FetchRemoteFileSafelyWithTimeout(ctx, rawURL, maxSize, timeout)
	}
	return utility.FetchRemoteFileSafelyWithValidator(ctx, rawURL, maxSize, timeout,
		func(ctx context.Context, rawURL string) (string, string, error) {
			return networks.AssertURLSafe(rawURL, func(host string) ([]string, error) {
				return net.DefaultResolver.LookupHost(ctx, host)
			})
		})
}
