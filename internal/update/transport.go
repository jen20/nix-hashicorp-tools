package update

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// Transport sets a User-Agent on every request and retries transient failures.
//
// The releases client makes its list and single-release requests through
// http.DefaultClient rather than the client configured via WithHTTPClient, and
// sets no User-Agent on them, so installing this as http.DefaultTransport is
// the only way to cover those requests as well as our own.
type Transport struct {
	Base      http.RoundTripper
	UserAgent string
	Attempts  int
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	attempts := max(t.Attempts, 1)

	if t.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		// The request belongs to the caller, so set the header on a copy.
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.UserAgent)
	}

	var lastErr error
	for attempt := range attempts {
		if attempt > 0 {
			if err := sleep(req, backoff(attempt)); err != nil {
				return nil, err
			}
		}

		resp, err := base.RoundTrip(req)
		if err != nil {
			lastErr = err
			continue
		}
		if !retryable(resp.StatusCode) || attempt == attempts-1 {
			return resp, nil
		}

		wait := backoff(attempt + 1)
		if after := retryAfter(resp); after > 0 {
			wait = after
		}
		_ = resp.Body.Close()

		if err := sleep(req, wait); err != nil {
			return nil, err
		}
		lastErr = fmt.Errorf("http %d", resp.StatusCode)
	}

	return nil, lastErr
}

// All requests made by the updater are bodyless GETs, so replaying one is
// always safe.
func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryAfter(resp *http.Response) time.Duration {
	seconds, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || seconds < 0 {
		return 0
	}
	return min(time.Duration(seconds)*time.Second, 30*time.Second)
}

func backoff(attempt int) time.Duration {
	return time.Duration(math.Pow(2, float64(attempt-1))) * 250 * time.Millisecond
}

func sleep(req *http.Request, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case <-timer.C:
		return nil
	}
}
