package handler

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LocalOnly is a middleware that restricts access to localhost only
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the client's IP address
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Check if the IP is localhost
		if ip != "127.0.0.1" && ip != "::1" && ip != "localhost" {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecureFileServer is a middleware that adds security headers and prevents directory listing
func SecureFileServer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Disposition", "attachment")

		// Prevent directory listing
		if strings.HasSuffix(r.URL.Path, "/") {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		// Only allow .zip files
		if !strings.HasSuffix(strings.ToLower(r.URL.Path), ".zip") {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecureAssetsServer is a middleware that adds security headers and restricts file types for assets
func SecureAssetsServer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Prevent directory listing
		if strings.HasSuffix(r.URL.Path, "/") {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		// Only allow .json and .jpg files
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		if ext != ".json" && ext != ".jpg" {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		// Set appropriate content type
		switch ext {
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		case ".jpg":
			w.Header().Set("Content-Type", "image/jpeg")
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimiter implements rate limiting using token bucket algorithm
type RateLimiter struct {
	ips    map[string]*rate.Limiter
	mu     *sync.RWMutex
	rps    float64
	burst  int
	ticker *time.Ticker
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	limiter := &RateLimiter{
		ips:    make(map[string]*rate.Limiter),
		mu:     &sync.RWMutex{},
		rps:    rps,
		burst:  burst,
		ticker: time.NewTicker(1 * time.Hour),
	}

	// Start cleanup routine
	go limiter.cleanup()

	return limiter
}

// cleanup removes old rate limiters periodically
func (rl *RateLimiter) cleanup() {
	for range rl.ticker.C {
		rl.mu.Lock()
		for ip := range rl.ips {
			delete(rl.ips, ip)
		}
		rl.mu.Unlock()
	}
}

// getLimiter returns a rate limiter for the given IP
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(rl.rps), rl.burst)
		rl.ips[ip] = limiter
	}

	return limiter
}

// RateLimit middleware limits requests per IP
func (rl *RateLimiter) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Close stops the cleanup routine
func (rl *RateLimiter) Close() {
	rl.ticker.Stop()
}