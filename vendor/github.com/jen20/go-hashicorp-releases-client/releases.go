package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

// Release returns all metadata for a specific version of a product.
func (c *Client) Release(ctx context.Context, product string, version string) (ReleaseInfo, error) {
	return c.singleRelease(ctx, c.makeURL(path.Join("v1", "releases", product, version), nil))
}

// LatestRelease returns all metadata for the latest release of a product with the given
// license class. If licenseClass is nil, the latest version of any license class is returned.
func (c *Client) LatestRelease(ctx context.Context, product string, licenseClass *LicenseClass) (ReleaseInfo, error) {
	query := url.Values{}
	if licenseClass != nil {
		query["license_class"] = []string{string(*licenseClass)}
	}

	return c.singleRelease(ctx, c.makeURL(path.Join("v1", "releases", product, "latest"), query))
}

func (c *Client) singleRelease(ctx context.Context, url url.URL) (ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return ReleaseInfo{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("%w: %d", ErrInvalidStatusCode, resp.StatusCode)
	}

	var target ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&target); err != nil {
		return ReleaseInfo{}, fmt.Errorf("%w: %w", ErrInvalidResponseBody, err)
	}

	return target, nil
}

// Releases returns an iter.Seq2 with an element for each release of the nominated product and
// license class. When ranging over the returned sequence, the second parameter may be an error,
// which should be guarded against in each loop iteration.
//
// See ExampleClient_Releases for further information on how to use the result of this function.
func (c *Client) Releases(ctx context.Context, product string, licenseClass *LicenseClass) (iter.Seq2[ReleaseInfo, error], error) {
	pages, err := c.ReleasesPaged(ctx, product, licenseClass)
	if err != nil {
		return nil, err
	}

	return func(yield func(ReleaseInfo, error) bool) {
		for page, err := range pages {
			if err != nil {
				_ = yield(ReleaseInfo{}, err)
				return
			}

			for _, item := range page {
				if !yield(item, nil) {
					return
				}
			}
		}
	}, nil
}

// ReleasesPaged returns an iter.Seq2 with an element for each page of releases of the nominated
// product and license class. When ranging over the returned sequence, the second parameter may
// be an error, which should be guarded against in each loop iteration.
//
// See ExampleClient_ReleasesPaged for further information on how to use the result of this function.
func (c *Client) ReleasesPaged(ctx context.Context, product string, licenseClass *LicenseClass) (iter.Seq2[[]ReleaseInfo, error], error) {
	if product == "" {
		return nil, fmt.Errorf("%w: may not be empty", ErrInvalidProduct)
	}

	switch licenseClass {
	case nil, LicenseClassAny, LicenseClassOSS, LicenseClassEnterprise, LicenseClassHCP:
		break
	default:
		return nil, fmt.Errorf("%w: must be one of %s", ErrInvalidLicenseClass, licenseClassNames())
	}

	paginator := &releasePaginator{
		client:         c,
		productURL:     c.makeURL(path.Join("v1", "releases", product), nil),
		pageSize:       16,
		licenseClass:   licenseClass,
		paginationMark: nil,
	}
	return paginator.iterator(ctx), nil
}

type releasePaginator struct {
	client         *Client
	productURL     url.URL
	pageSize       int
	licenseClass   *LicenseClass
	paginationMark *time.Time
}

func (r *releasePaginator) iterator(ctx context.Context) iter.Seq2[[]ReleaseInfo, error] {
	return func(yield func([]ReleaseInfo, error) bool) {
		for {
			page, err := r.requestPage(ctx)
			if err != nil {
				_ = yield(nil, err)
				break
			}

			if len(page) == 0 {
				break
			}
			yield(page, nil)

			r.paginationMark = &page[len(page)-1].TimestampCreated
		}
	}
}

func (r *releasePaginator) requestPage(ctx context.Context) ([]ReleaseInfo, error) {
	query := url.Values{
		"limit": []string{strconv.Itoa(r.pageSize)},
	}
	if r.paginationMark != nil {
		query["after"] = []string{(*r.paginationMark).Format(time.RFC3339)}
	}
	if r.licenseClass != nil {
		query["license_class"] = []string{string(*r.licenseClass)}
	}

	pageURL := r.productURL
	pageURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrInvalidStatusCode, resp.StatusCode)
	}

	var target []ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&target); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidResponseBody, err)
	}

	return target, nil
}
