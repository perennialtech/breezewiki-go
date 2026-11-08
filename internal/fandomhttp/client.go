// Package fandomhttp provides HTTP transport compatible with Fandom's edge.
package fandomhttp

import (
	"fmt"
	"net/http"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// NewClient returns a conventional net/http client with a browser TLS and
// HTTP/2 profile. Fandom's Cloudflare edge rejects standard Go TLS clients.
func NewClient() (*http.Client, error) {
	client, err := tlsclient.NewHttpClient(nil,
		tlsclient.WithClientProfile(profiles.Chrome_150),
		tlsclient.WithNotFollowRedirects(),
		tlsclient.WithTimeoutSeconds(15),
	)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: &roundTripper{client: client}, Timeout: 15 * time.Second}, nil
}

type roundTripper struct {
	client tlsclient.HttpClient
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	upstream, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	upstream.Header = fhttp.Header(req.Header.Clone())
	upstream.Host = req.Host
	upstream.ContentLength = req.ContentLength
	resp, err := t.client.Do(upstream)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		Status:           resp.Status,
		StatusCode:       resp.StatusCode,
		Proto:            resp.Proto,
		ProtoMajor:       resp.ProtoMajor,
		ProtoMinor:       resp.ProtoMinor,
		Header:           http.Header(resp.Header),
		Body:             resp.Body,
		ContentLength:    resp.ContentLength,
		TransferEncoding: resp.TransferEncoding,
		Request:          req,
	}, nil
}

func (t *roundTripper) CloseIdleConnections() {
	t.client.CloseIdleConnections()
}
