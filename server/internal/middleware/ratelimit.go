// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package middleware

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty.go/swifty_http"
)

// RateLimit throttles an endpoint per client IP: at most max requests within
// each sliding window. It guards the unauthenticated endpoints (login,
// register, password reset) against credential-stuffing and brute-force
// attempts.
func RateLimit(max int, window time.Duration) swifty_http.Middleware {
	var mu sync.Mutex
	hits := make(map[string][]time.Time)
	return func(ctx *swifty_http.Context, next func()) {
		ip := clientIP(ctx)
		now := time.Now()
		mu.Lock()
		recent := hits[ip][:0]
		for _, t := range hits[ip] {
			if now.Sub(t) < window {
				recent = append(recent, t)
			}
		}
		if len(recent) >= max {
			hits[ip] = recent
			mu.Unlock()
			ctx.Status = 200
			ctx.JSON(swifty_http.H{"code": 429, "message": "too many requests, slow down"})
			return
		}
		hits[ip] = append(recent, now)
		mu.Unlock()
		next()
	}
}

// clientIP prefers the first X-Forwarded-For entry (set by a trusted proxy)
// and falls back to the connection's remote address.
func clientIP(ctx *swifty_http.Context) string {
	if fwd := ctx.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(ctx.Request.RemoteAddr)
	if err != nil {
		return ctx.Request.RemoteAddr
	}
	return host
}
