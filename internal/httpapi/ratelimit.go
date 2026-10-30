package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/httprate"
)

const (
	// writeRateLimitWindow is the window the write limit is rolled over: at most writeRateLimit
	// POST /bookmarks requests in any 60-second span.
	writeRateLimitWindow = time.Minute
	// writeRateLimitMessage is the message a request over the write limit is answered with.
	writeRateLimitMessage = "too many requests"
)

// clientIPKey keys the write limit on the connection's address. It deliberately does not use
// httprate's default key function, which consults X-Real-IP and X-Forwarded-For: those headers are
// set by the client, so trusting them would let one client pick its own bucket. The port is
// dropped, because a client's source port changes from one connection to the next.
func clientIPKey(r *http.Request) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// No port to split off, so the address is the whole of RemoteAddr.
		return r.RemoteAddr, nil
	}
	return host, nil
}

// tooManyRequests answers a request over the write limit with the project's error shape and a
// Retry-After header saying how long the client has to wait: the whole window, in seconds, which
// is a positive integer.
func tooManyRequests(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Retry-After", strconv.FormatInt(int64(writeRateLimitWindow/time.Second), 10))
	writeError(w, http.StatusTooManyRequests, writeRateLimitMessage)
}

// writeRateLimiter returns the middleware that allows limit POST /bookmarks requests per client IP
// per writeRateLimitWindow. A limit of zero or less returns a middleware that passes every request
// through and allocates no counters.
//
// The counters live in this process, which is what a single server needs; running more than one
// instance would need a shared limiter instead.
func writeRateLimiter(limit int) func(http.Handler) http.Handler {
	if limit <= 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return httprate.Limit(
		limit,
		writeRateLimitWindow,
		httprate.WithKeyFuncs(clientIPKey),
		httprate.WithLimitHandler(tooManyRequests),
	)
}
