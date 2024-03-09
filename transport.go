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

type ShouldRetryFunc func(*http.Request, *http.Response, error) bool
type NotifyFunc func(ctx context.Context, err error, duration time.Duration)
type BackOffPolicy struct {
	MaxRetries uint64
}
type RoundTripper struct {
	roundTripper    http.RoundTripper
	shouldRetryFunc ShouldRetryFunc
	notifyFunc      NotifyFunc
	backOffPolicy   *BackOffPolicy
}

var ShouldRetryRespError = errors.New("should retry response error")

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
