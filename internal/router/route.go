// Package router handles HTTP routing
package router

import (
	"net"
	"time"
)

// Route represents a single routing entry
type Route struct {
	ID          string
	Host        string
	PathPrefix  string
	Backend     *BackendPool
	TLS         TLSConfig
	Middlewares []string
	Priority    int
	CreatedAt   time.Time
	Labels      map[string]string

	// Container info
	ContainerID   string
	ContainerName string

	// Additional fields for routing
	Address string // Direct backend address

	// Per-route middleware configuration
	MiddlewareConfig MiddlewareConfig
}

// MiddlewareConfig holds per-route middleware settings
type MiddlewareConfig struct {
	// Rate limiting
	RateLimit RateLimitConfig

	// CORS
	CORS CORSConfig

	// Compression
	Compress bool

	// Path modification
	StripPrefix string
	AddPrefix   string

	// Security
	BasicAuthUsers []BasicAuthUser
	IPWhitelist    []*net.IPNet
	IPBlacklist    []*net.IPNet
	MaxBody        int64

	// Reliability
	Retry          int
	CircuitBreaker CircuitBreakerConfig
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled bool
	Count   int
	Window  time.Duration
	ByKey   string
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled bool
	Origins []string
	Methods []string
	Headers []string
}

// BasicAuthUser holds basic auth credentials
type BasicAuthUser struct {
	Username string
	Hash     string
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled  bool
	Failures int
	Window   time.Duration
}

// TLSConfig holds TLS-related configuration for a route
type TLSConfig struct {
	Mode     string   // auto, manual, off
	Domains  []string // SAN domains
	CertFile string
	KeyFile  string
}

// IsEnabled returns true if TLS is enabled
func (t *TLSConfig) IsEnabled() bool {
	return t.Mode != "off"
}

// IsAuto returns true if auto TLS (ACME) is enabled
func (t *TLSConfig) IsAuto() bool {
	return t.Mode == "auto"
}

// Clone creates a deep copy of the route
func (r *Route) Clone() *Route {
	cp := *r

	// Deep copy slices
	if r.Middlewares != nil {
		cp.Middlewares = make([]string, len(r.Middlewares))
		copy(cp.Middlewares, r.Middlewares)
	}
	if r.Labels != nil {
		cp.Labels = make(map[string]string, len(r.Labels))
		for k, v := range r.Labels {
			cp.Labels[k] = v
		}
	}
	if r.TLS.Domains != nil {
		cp.TLS.Domains = make([]string, len(r.TLS.Domains))
		copy(cp.TLS.Domains, r.TLS.Domains)
	}

	// Deep copy MiddlewareConfig slices
	if r.MiddlewareConfig.BasicAuthUsers != nil {
		cp.MiddlewareConfig.BasicAuthUsers = make([]BasicAuthUser, len(r.MiddlewareConfig.BasicAuthUsers))
		copy(cp.MiddlewareConfig.BasicAuthUsers, r.MiddlewareConfig.BasicAuthUsers)
	}
	if r.MiddlewareConfig.IPWhitelist != nil {
		cp.MiddlewareConfig.IPWhitelist = make([]*net.IPNet, len(r.MiddlewareConfig.IPWhitelist))
		for i, cidr := range r.MiddlewareConfig.IPWhitelist {
			cpy := *cidr
			cp.MiddlewareConfig.IPWhitelist[i] = &cpy
		}
	}
	if r.MiddlewareConfig.IPBlacklist != nil {
		cp.MiddlewareConfig.IPBlacklist = make([]*net.IPNet, len(r.MiddlewareConfig.IPBlacklist))
		for i, cidr := range r.MiddlewareConfig.IPBlacklist {
			cpy := *cidr
			cp.MiddlewareConfig.IPBlacklist[i] = &cpy
		}
	}
	if r.MiddlewareConfig.CORS.Origins != nil {
		cp.MiddlewareConfig.CORS.Origins = make([]string, len(r.MiddlewareConfig.CORS.Origins))
		copy(cp.MiddlewareConfig.CORS.Origins, r.MiddlewareConfig.CORS.Origins)
	}
	if r.MiddlewareConfig.CORS.Methods != nil {
		cp.MiddlewareConfig.CORS.Methods = make([]string, len(r.MiddlewareConfig.CORS.Methods))
		copy(cp.MiddlewareConfig.CORS.Methods, r.MiddlewareConfig.CORS.Methods)
	}
	if r.MiddlewareConfig.CORS.Headers != nil {
		cp.MiddlewareConfig.CORS.Headers = make([]string, len(r.MiddlewareConfig.CORS.Headers))
		copy(cp.MiddlewareConfig.CORS.Headers, r.MiddlewareConfig.CORS.Headers)
	}

	return &cp
}
