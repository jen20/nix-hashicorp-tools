package releases

import (
	"net/http"
	"net/url"
	"path"
)

var (
	defaultUserAgent = "go-hashicorp-releases-client"
	defaultBaseURL   = url.URL{
		Scheme: "https",
		Host:   "api.releases.hashicorp.com",
	}
)

// ClientOpt is a functional option which can be used to configure a Client via the New function.
type ClientOpt func(*clientOpts) error

// Client provides a handle to interact with the HashiCorp Releases API.
type Client struct {
	opts clientOpts
}

// New creates a new Client, and uses the supplied options to configure it.
func New(opts ...ClientOpt) (*Client, error) {
	effectiveOpts, err := newClientOpts(opts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		opts: effectiveOpts,
	}, nil
}

func (c *Client) makeURL(pathComponents string, query url.Values) url.URL {
	result := c.opts.baseURL
	result.Path = path.Join(result.Path, pathComponents)
	result.RawQuery = query.Encode()
	return result
}

type clientOpts struct {
	httpClient *http.Client
	userAgent  *string
	baseURL    url.URL
}

func newClientOpts(opts ...ClientOpt) (clientOpts, error) {
	effectiveOpts := clientOpts{
		httpClient: http.DefaultClient,
		userAgent:  &defaultUserAgent,
		baseURL:    defaultBaseURL,
	}

	for _, opt := range opts {
		if err := opt(&effectiveOpts); err != nil {
			return clientOpts{}, err
		}
	}
	return effectiveOpts, nil
}

// WithHTTPClient configures a custom [http.Client] with which to make requests. If this
// option is not supplied, or httpClient is set to nil, [http.DefaultClient] will be used.
func WithHTTPClient(httpClient *http.Client) ClientOpt {
	return func(opts *clientOpts) error {
		if httpClient != nil {
			opts.httpClient = httpClient
		}
		return nil
	}
}

// WithUserAgent configures the value of the HTTP User-Agent header to send with requests.
// If this option is not set, "go-hashicorp-releases-client vX.Y.Z" will be used, where X.Y.Z
// is the version of this library in use.
//
// This should be set to the name and version of the consuming application, however if you
// wish to disable sending a value for User-Agent, supply the WithoutUserAgent option.
func WithUserAgent(userAgent string) ClientOpt {
	return func(opts *clientOpts) error {
		opts.userAgent = &userAgent
		return nil
	}
}

// WithoutUserAgent disables sending an HTTP User-Agent header with requests.
//
// Use of this option is discouraged - applications should instead use WithUserAgent, and
// supply the name and version of the consuming application.
func WithoutUserAgent() ClientOpt {
	return func(opts *clientOpts) error {
		opts.userAgent = nil
		return nil
	}
}

// WithBaseURL sets the URL at which the root of the releases API may be reached.
//
// If unset, this is set to https://api.releases.hashicorp.com.
func WithBaseURL(baseURL string) ClientOpt {
	return func(opts *clientOpts) error {
		parsed, err := url.Parse(baseURL)
		if err != nil {
			return err
		}
		opts.baseURL = *parsed
		return nil
	}
}
