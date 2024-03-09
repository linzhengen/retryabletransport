package retryabletransport_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/linzhengen/retryabletransport"
	"github.com/stretchr/testify/assert"
)

type mock struct {
	mockErr  error
	mockResp *http.Response
}

func (m *mock) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if m.mockErr == nil {
		return resp, err
	}
	return resp, m.mockErr
}

func Test_RoundTripper_RoundTrip(t *testing.T) {
	type oneHeader struct {
		key   string
		value string
	}
	type test struct {
		name          string
		err           error
		resp          *http.Response
		requestBody   string
		requestHeader *oneHeader
		retriedCount  uint64
	}
	tests := []test{
		{
			name:          "ECONNRESET(connection reset by peer) should retry",
			err:           syscall.ECONNRESET,
			resp:          nil,
			requestBody:   `body1`,
			requestHeader: &oneHeader{key: "Foo", value: "bar"},
			retriedCount:  uint64(1),
		},
		{
			name:          "StatusInternalServerError should not retry",
			err:           nil,
			resp:          &http.Response{StatusCode: http.StatusInternalServerError},
			requestBody:   `body2`,
			requestHeader: &oneHeader{key: "Foo", value: "fee"},
			retriedCount:  uint64(0),
		},
		{
			name:          "http request success",
			err:           nil,
			resp:          &http.Response{StatusCode: http.StatusOK},
			requestBody:   "body3",
			requestHeader: &oneHeader{key: "Foo", value: "foo"},
			retriedCount:  uint64(0),
		},
		{
			name:          "StatusTooManyRequests should retry",
			err:           retryabletransport.ShouldRetryRespError,
			resp:          &http.Response{StatusCode: http.StatusTooManyRequests},
			requestBody:   `body4`,
			requestHeader: nil,
			retriedCount:  uint64(1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{
				Transport: retryabletransport.New(
					&mock{
						mockErr:  tc.err,
						mockResp: tc.resp,
					},
					func(req *http.Request, resp *http.Response, err error) bool {
						if errors.Is(err, syscall.ECONNRESET) {
							return true
						}
						if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
							t.Logf("retry http request, status code: %d", resp.StatusCode)
							return true
						}
						return false
					},
					func(ctx context.Context, err error, duration time.Duration) {
						t.Logf("retry http request, err: %v, duration: %v", err, duration)
					},
					&retryabletransport.BackOffPolicy{
						MaxRetries: tc.retriedCount,
					},
				),
				Timeout: 2 * time.Second,
			}
			calledCount := uint64(0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.resp != nil {
					w.WriteHeader(tc.resp.StatusCode)
				}
				requestBody, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				defer func(Body io.ReadCloser) {
					if err := Body.Close(); err != nil {
						t.Fatal(err)
					}
				}(r.Body)
				assert.Equal(t, tc.requestBody, string(requestBody))
				if tc.requestHeader != nil {
					assert.Equal(t, tc.requestHeader.value, r.Header.Get(tc.requestHeader.key))
				}
				atomic.AddUint64(&calledCount, 1)
			}))
			req, err := http.NewRequest("POST", server.URL, strings.NewReader(tc.requestBody))
			if err != nil {
				t.Fatal(err)
			}
			if tc.requestHeader != nil {
				req.Header.Set(tc.requestHeader.key, tc.requestHeader.value)
			}
			resp, err := client.Do(req)
			if !errors.Is(err, tc.err) {
				assert.ErrorIs(t, err, tc.err)
			}
			assert.Equal(t, tc.retriedCount+1, calledCount)
			if resp != nil {
				assert.Equal(t, tc.resp.StatusCode, resp.StatusCode)
			}
		})
	}
}
