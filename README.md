# Retryable Transport
The retryabletransport package provides a customizable HTTP transport mechanism with retry functionality built-in. This can be particularly useful for scenarios where network requests might fail intermittently due to transient errors, and retrying them can increase the chances of success.

## Features
- Customizable Retries: Users can specify a custom retry policy through the ShouldRetryFunc type, which determines whether a request should be retried based on the HTTP request, response, and error encountered.
- Backoff Strategy: The package utilizes an exponential backoff strategy for retrying requests, which progressively increases the time between retries to mitigate overloading the server with retry attempts.
- Notification: Users can optionally provide a NotifyFunc to receive notifications about retry attempts, including the error encountered and the duration between retries.
- Configurable Maximum Retries: The BackOffPolicy struct allows users to set the maximum number of retries for a given request.

## Usage
```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"syscall"
	"time"

	"github.com/linzhengen/retryabletransport"
)

func main() {
	client := &http.Client{
		Transport: retryabletransport.New(
			http.DefaultTransport,
			func(req *http.Request, resp *http.Response, err error) bool {
				if errors.Is(err, syscall.ECONNRESET) {
					return true
				}
				if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
					return true
				}
				return false
			},
			func(ctx context.Context, err error, duration time.Duration) {
				fmt.Printf("retry http request, err: %v, duration: %v", err, duration)
			},
			&retryabletransport.BackOffPolicy{
				MaxRetries: 3,
			}),
	),
		Timeout: 3 * time.Second,
	}
	_, err := client.Get("http://example.com")
	if err != nil {
		fmt.Println(err)
	}
}
```