package connector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

func stubPrivateNetworkDNS(t *testing.T, hosts map[string][]string) {
	t.Helper()
	lookup := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		if answers, ok := hosts[host]; ok {
			return answers, nil
		}
		if net.ParseIP(host) != nil {
			return []string{host}, nil
		}
		return nil, fmt.Errorf("unexpected DNS lookup for %q", host)
	}
	t.Cleanup(func() { common.LookupHost = lookup })
}

func TestPrivateNetworksApplyToConnectorURLAndTCP(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "1")
	t.Setenv(allowedPrivateNetworksEnv, `["10.42.16.12","10.42.20.0/24","fd00::/8","100.64.0.0/10"]`)
	stubPrivateNetworkDNS(t, map[string][]string{
		"inside.example.test": {"10.42.20.2", "8.8.8.8", "10.42.20.3"},
		"mixed.example.test":  {"10.42.20.2", "10.42.21.3"},
	})
	for _, tc := range []struct {
		host, pin string
	}{
		{"10.42.16.12", "10.42.16.12"},
		{"10.42.16.13", ""},
		{"10.42.20.254", "10.42.20.254"},
		{"10.42.21.1", ""},
		{"::ffff:10.42.20.5", "10.42.20.5"},
		{"fd12:3456::12", "fd12:3456::12"},
		{"8.8.8.8", "8.8.8.8"},
		{"inside.example.test", "10.42.20.2"},
		{"mixed.example.test", ""},
		{"127.0.0.1", ""},
		{"::ffff:127.0.0.1", ""},
		{"169.254.169.254", ""},
		{"fe80::1", ""},
		{"100.100.100.200", ""},
		{"fd00:ec2::254", ""},
	} {
		t.Run(tc.host, func(t *testing.T) {
			target := "https://" + net.JoinHostPort(tc.host, "443")
			_, urlPin, urlErr := assertConnectorURLSafe(target)
			tcpPin, tcpErr := assertConnectorHostSafe(tc.host)
			if tc.pin == "" {
				if urlErr == nil || tcpErr == nil {
					t.Fatalf("unexpected allow: URL=%v TCP=%v", urlPin, tcpPin)
				}
				return
			}
			if urlErr != nil || tcpErr != nil || urlPin.String() != tc.pin || tcpPin.String() != tc.pin {
				t.Fatalf("URL=%v (%v) TCP=%v (%v), want %s", urlPin, urlErr, tcpPin, tcpErr, tc.pin)
			}
		})
	}
	t.Setenv(common.EnvAllowAnyHost, "")
	if _, _, err := common.AssertURLSafe("https://10.42.16.12"); err == nil {
		t.Fatal("connector allowlist leaked into the shared URL guard")
	}
	if err := validateSitemapURL("https://10.42.16.12/sitemap.xml"); err != nil {
		t.Fatalf("Sitemap did not honor connector allowlist: %v", err)
	}
}

func TestPrivateNetworksRejectInvalidConfig(t *testing.T) {
	for _, raw := range []string{
		`null`, `"10.42.16.12"`, `[123]`, `["10.42.16.12","invalid"]`,
		`["10.42.20.0/33"]`, `["10.0.0.0/7"]`, `["::/0"]`, `["127.0.0.1"]`, `["169.254.0.0/16"]`,
	} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(allowedPrivateNetworksEnv, raw)
			if _, _, err := assertConnectorURLSafe("https://10.42.16.12"); err == nil || !strings.Contains(err.Error(), allowedPrivateNetworksEnv) {
				t.Fatalf("invalid config accepted or not reported: %v", err)
			}
		})
	}
}

func TestPrivateNetworksSharedByConnectorFactories(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "")
	t.Setenv(allowedPrivateNetworksEnv, `["10.42.20.0/24"]`)
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	for _, tc := range []struct {
		ip   string
		want bool
	}{{"10.42.20.5", true}, {"10.42.21.5", false}} {
		config := entity.JSONMap{"url": "https://" + tc.ip + "/api", "content_fields": "title"}
		_, apiErr := registry.OpenFromConfig("rest_api", config)
		_, syncErr := registry.Open(t.Context(), dao.SyncTaskContext{
			Connector: entity.Connector{Source: "rest_api", Config: entity.ConnectorConfig(config)},
		})
		if (apiErr == nil) != tc.want || (syncErr == nil) != tc.want {
			t.Errorf("%s: API=%v Syncer=%v want accepted=%t", tc.ip, apiErr, syncErr, tc.want)
		}
	}
}

func TestPrivateNetworksPinHTTPAndRecheckRedirects(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "")
	t.Setenv(allowedPrivateNetworksEnv, `["10.42.20.0/24"]`)
	stubPrivateNetworkDNS(t, map[string][]string{"docs.example.test": {"10.42.20.5"}})
	requests := make(chan string, 2)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r.URL.Host:
		default:
		}
		http.Redirect(w, r, "http://10.42.21.5/outside", http.StatusFound)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := connectorRequest(t.Context(), connectorRequestOptions{
		Method:  http.MethodGet,
		RawURL:  "http://docs.example.test:8123/file",
		Timeout: time.Second,
		Base:    &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}},
	})
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "10.42.21.5") {
		t.Fatalf("redirect outside allowlist was not rejected: %v", err)
	}
	select {
	case host := <-requests:
		if host != "10.42.20.5:8123" {
			t.Fatalf("proxy target %q was not pinned", host)
		}
	default:
		t.Fatal("approved address was not reached")
	}
	if len(requests) != 0 {
		t.Fatal("denied redirect reached proxy")
	}
}

func TestPrivateNetworksApplyToConnectorDownloads(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "")
	t.Setenv(allowedPrivateNetworksEnv, `["10.42.20.0/24"]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.42.21.5/outside", http.StatusFound)
	}))
	defer server.Close()
	pinned := utility.PinnedHTTPClient
	utility.PinnedHTTPClient = func(host, ip string, timeout time.Duration) *http.Client {
		if host != "10.42.20.5" || ip != host {
			t.Errorf("download attempted to use an unapproved pin: %q (%q)", host, ip)
		}
		return &http.Client{
			Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}},
			Timeout: timeout,
		}
	}
	t.Cleanup(func() { utility.PinnedHTTPClient = pinned })

	_, _, _, err := fetchConnectorRemoteFile(t.Context(), "http://10.42.20.5/file", 1024, time.Second)
	if err == nil || !strings.Contains(err.Error(), "10.42.21.5") {
		t.Fatalf("download redirect outside allowlist was not rejected: %v", err)
	}
}

func TestPrivateNetworksApplyToAzureBlobConnectionString(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "")
	t.Setenv(allowedPrivateNetworksEnv, `["10.42.20.0/24"]`)
	for _, tc := range []struct {
		ip   string
		want bool
	}{{"10.42.20.5", true}, {"10.42.21.5", false}} {
		c, err := NewAzureBlobStorageConnector(map[string]any{"credentials": map[string]any{
			"container_name": "fixture",
			"connection_string": "DefaultEndpointsProtocol=https;AccountName=fixture;" +
				"AccountKey=Zml4dHVyZQ==;BlobEndpoint=https://" + tc.ip,
		}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.ensureClient(t.Context())
		if (err == nil) != tc.want {
			t.Errorf("%s: Azure Blob client error=%v want accepted=%t", tc.ip, err, tc.want)
		}
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://10.42.21.5/blob", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (azureBlobTransport{}).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("Azure Blob SDK transport accepted a denied destination: %v", err)
	}
}
