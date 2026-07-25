package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
)

// Products returns a slice of the HashiCorp products for which release information may be obtained.
func (c *Client) Products(ctx context.Context) ([]string, error) {
	const mediaType = "application/vnd+hashicorp.releases-api.v1+json"

	reqURL := c.makeURL(path.Join("v1", "products"), nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConstructingRequest, err)
	}
	req.Header.Set("Accept", mediaType)
	if c.opts.userAgent != nil {
		req.Header.Set("User-Agent", *c.opts.userAgent)
	}

	resp, err := c.opts.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if contentType := resp.Header.Get("Content-Type"); contentType != mediaType {
		if contentType == "" {
			contentType = "<none>"
		}
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponseContentType, contentType)
	}

	var body []string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidResponseBody, err)
	}

	return body, nil
}
