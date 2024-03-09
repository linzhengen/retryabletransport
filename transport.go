package retryabletransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// ShouldRetryFunc represents a function that determines whether a request should be retried based on the request, response, and error.
type ShouldRetryFunc func(*http.Request, *http.Response, error) bool

// NotifyFunc represents a function that notifies about errors and durations during retries.
type NotifyFunc func(ctx context.Context, err error, duration time.Duration)

// BackOffPolicy represents the maximum number of retries for a backoff policy.
type BackOffPolicy struct {
	MaxRetries uint64
}

// RoundTripper provides a retryable HTTP transport mechanism.
type RoundTripper struct {
	roundTripper    http.RoundTripper
	shouldRetryFunc ShouldRetryFunc
	notifyFunc      NotifyFunc
	backOffPolicy   *BackOffPolicy
}

// ShouldRetryRespError is returned when a response indicates the request should be retried.
var ShouldRetryRespError = errors.New("should retry response error")

// New creates a new RoundTripper with the provided parameters. If roundTripper is nil, http.DefaultTransport is used.
// If backOffPolicy is nil, a default policy with MaxRetries set to 3 is used.
func New(roundTripper http.RoundTripper, shouldRetryFunc ShouldRetryFunc, notifyFunc NotifyFunc, backOffPolicy *BackOffPolicy) *RoundTripper {
	if roundTripper == nil {
		roundTripper = http.DefaultTransport
	}
	if backOffPolicy == nil {
		backOffPolicy = &BackOffPolicy{MaxRetries: 3}
	}
	return &RoundTripper{
		backOffPolicy:   backOffPolicy,
		roundTripper:    roundTripper,
		shouldRetryFunc: shouldRetryFunc,
		notifyFunc:      notifyFunc,
	}
}

// RoundTrip executes a single HTTP transaction and returns a response.
// It implements the http.RoundTripper interface.
func (p *RoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	bodyByte, err := readBody(req)
	if err != nil {
		return nil, err
	}
	b := backoff.NewExponentialBackOff()
	err = backoff.RetryNotify(func() error {
		req.Body = io.NopCloser(bytes.NewReader(bodyByte))
		resp, err = p.roundTripper.RoundTrip(req)
		if p.shouldRetryFunc(req, resp, err) {
			if err == nil {
				return ShouldRetryRespError
			}
			return err
		}
		return backoff.Permanent(err)
	},
		backoff.WithMaxRetries(b, p.backOffPolicy.MaxRetries),
		func(err error, duration time.Duration) {
			if p.notifyFunc != nil {
				p.notifyFunc(req.Context(), err, duration)
			}
		},
	)
	return resp, err
}

// readBody reads the request body and closes it, returning the body as a byte slice.
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err = r.Body.Close(); err != nil {
		return nil, err
	}
	return b, nil
}
