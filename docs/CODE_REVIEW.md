# DockRouter Comprehensive Code Review Report

**Date**: 2026-05-02
**Reviewer**: Automated Forensic Audit (Claude Code)
**Scope**: All 52 production Go source files, ~15,000+ LOC
**Go Version**: 1.22 | **Dependencies**: Zero (stdlib only)

---

## Executive Summary

DockRouter is a Docker-native reverse proxy with automatic container discovery, TLS certificate provisioning via ACME, load balancing, and a rich middleware stack. The codebase demonstrates strong architectural separation, zero external dependencies, and proper use of Go idioms in many areas. However, the review identified **13 CRITICAL**, **21 HIGH**, **34 MEDIUM**, and **18 LOW** severity issues across the entire codebase.

The most impactful issues are: (1) the retry/failover logic in the router is dead code — after the first backend failure, the response writer is already consumed, making retries impossible; (2) the on-demand TLS provisioning always fails the first client's TLS handshake for any new domain; (3) the WebSocket proxy does not validate backend responses, enabling response injection attacks; and (4) the ACME client has a nonce race condition that breaks concurrent certificate provisioning.

## Risk Assessment

**Overall Risk Level**: HIGH

The project is architecturally sound but has several production-critical bugs that undermine its core value propositions (failover, TLS provisioning, WebSocket proxying). The security posture is reasonable for an internal tool but has notable gaps (open redirect, XFF injection, CORS misconfiguration) that would need addressing before public-facing deployment.

## Metrics

| Metric | Score |
|--------|-------|
| Code Health | 6/10 |
| Security Score | 5/10 |
| Concurrency Safety | 4/10 |
| Maintainability | 7/10 |
| Test Coverage | ~60% estimated |

## Issue Summary

| Severity | Count |
|----------|-------|
| CRITICAL | 13 |
| HIGH | 21 |
| MEDIUM | 34 |
| LOW | 18 |
| **Total** | **86** |

---

## Top 10 Critical Issues

1. **Dead retry/failover logic** — `ResponseWriter` consumed after first failure, making retries impossible
2. **On-demand TLS always fails first request** — goroutine provisioning returns error immediately
3. **WebSocket backend response not validated** — raw bytes forwarded without HTTP parsing
4. **WebSocket response may be truncated** — single `Read()` doesn't guarantee complete response
5. **ACME nonce race condition** — concurrent provisioning corrupts shared nonce field
6. **X-Forwarded-For header injection** — client-supplied XFF header blindly forwarded
7. **Reconciler race condition** — `AddRoute`/`RemoveRoute` called both inside and outside mutex
8. **Docker event stream goroutine leak** — `decoder.Decode()` blocks despite context cancellation
9. **Radix tree `sync.Pool` use-after-free risk** — recycled nodes may reference live tree data
10. **WebSocket goroutine leak** — function returns before backend→client copy goroutine completes

---

## CRITICAL FINDINGS

### C-01: Dead Retry/Failover Logic

**Category**: Logic Bug / Reliability
**File**: `internal/router/router.go:100-161`
**Impact**: After the first proxy attempt fails, `httputil.ReverseProxy` has already written headers and body to the `http.ResponseWriter`. The retry loop's `triedBackends` map and `maxRetries` parameter are dead code. Routes with multiple backends never fail over.

```go
// Line 153-155: Always returns after first failure
// The proxy's error handler already wrote the error response
// to the ResponseWriter, so we cannot retry with the same writer.
return
```

**Recommendation**: Use a buffered response writer (`httptest.ResponseRecorder`) to capture the first attempt, only flushing to the real writer on success.

---

### C-02: On-Demand TLS Returns Error on First Request

**Category**: Reliability
**File**: `internal/tls/manager.go:99-113`
**Impact**: The first TLS handshake for any new domain always fails. `GetCertificate` fires an async goroutine but immediately returns an error. Browsers display a certificate error.

```go
go func() {
    defer m.provisioning.Delete(domain)
    if err := m.EnsureCertificate(domain); err != nil { ... }
}()
return nil, fmt.Errorf("certificate not found for %s", domain)
```

**Recommendation**: Return a self-signed fallback certificate immediately (the `GenerateSelfSigned` function already exists), then swap in the real certificate once ACME provisioning completes.

---

### C-03: WebSocket Backend Response Not Validated

**Category**: Security
**File**: `internal/proxy/websocket.go:118-125`
**Impact**: `readBackendResponse` reads raw bytes without parsing the HTTP response. A malicious backend can send any response (not 101 Switching Protocols), enabling response splitting.

```go
func (wp *WebSocketProxy) readBackendResponse(conn net.Conn) (string, error) {
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    return string(buf[:n]), nil  // No validation
}
```

**Recommendation**: Use `http.ReadResponse` to parse and validate the 101 status code.

---

### C-04: WebSocket Response Truncation

**Category**: Reliability
**File**: `internal/proxy/websocket.go:118-125`
**Impact**: A single `conn.Read()` is not guaranteed to read the complete HTTP response. TCP may deliver the response in multiple segments, corrupting the WebSocket handshake.

**Recommendation**: Use `bufio.Reader` and read until `\r\n\r\n`.

---

### C-05: ACME Nonce Race Condition

**Category**: Concurrency
**File**: `internal/tls/acme.go:35,269,372-374`
**Impact**: `ACMEClient.nonce` is read/written by concurrent goroutines without synchronization. Concurrent certificate provisioning for different domains produces corrupted nonces, causing ACME "badNonce" errors.

```go
// signPayload reads nonce:
"nonce": c.nonce,
// signedPost writes nonce:
if nonce := resp.Header.Get("Replay-Nonce"); nonce != "" {
    c.nonce = nonce
}
```

**Recommendation**: Add `sync.Mutex` to `ACMEClient`, lock around nonce read+consume and update.

---

### C-06: X-Forwarded-For Header Injection

**Category**: Security
**File**: `internal/proxy/proxy.go:99-106`
**Impact**: Client-supplied `X-Forwarded-For` header is blindly forwarded with the real IP appended. Backends trusting XFF for access control or rate limiting will see forged IPs.

```go
xff := original.Header.Get("X-Forwarded-For")
if xff != "" {
    xff = xff + ", " + clientIP
}
```

**Recommendation**: Always overwrite with only the real client IP, or use a trusted proxy chain.

---

### C-07: Reconciler Race Condition on Route Updates

**Category**: Concurrency
**File**: `internal/discovery/reconciler.go:143-144, 316-319`
**Impact**: `Sync()` calls `AddRoute()` inside the mutex, but `onContainerStart()` releases the lock before calling `AddRoute()`. Concurrent calls from event and poll goroutines can corrupt the route table.

```go
// Sync() - inside lock:
e.mu.Lock()
e.routes.AddRoute(info)

// onContainerStart() - outside lock:
e.mu.Lock()
e.containers[id] = info
e.mu.Unlock()
e.routes.AddRoute(info)  // RACE
```

**Recommendation**: Consistently call route sink methods either always inside or always outside the lock, with the route table handling its own synchronization.

---

### C-08: Docker Event Stream Goroutine Leak

**Category**: Concurrency / Resource Leak
**File**: `internal/discovery/docker.go:316-328`
**Impact**: `decoder.Decode()` blocks indefinitely. The `ctx.Done()` check in `select/default` only runs between decode calls. If `Decode` blocks, the goroutine never exits.

```go
for {
    select {
    case <-ctx.Done():
        return
    default:
        var event Event
        if err := decoder.Decode(&event); err != nil {  // BLOCKS HERE
```

**Recommendation**: Use a separate goroutine for decoding with a result channel, and select between decode result and context cancellation.

---

### C-09: Radix Tree sync.Pool Use-After-Free Risk

**Category**: Concurrency / Data Integrity
**File**: `internal/router/radix.go:10-28`
**Impact**: `putNode` re-slices `children` (`n.children[:0]`) which keeps the old backing array. Recycled nodes via `sync.Pool` could retain references to live tree children. Currently `putNode` is defined but never called in the delete path, making it dead code that would cause corruption if activated.

**Recommendation**: Remove `nodePool`, `getNode`, and `putNode` entirely. Radix tree nodes have variable lifetimes unsuitable for `sync.Pool`.

---

### C-10: WebSocket Goroutine Leak

**Category**: Resource Leak
**File**: `internal/proxy/websocket.go:79-82`
**Impact**: The function launches a goroutine for backend→client copy and blocks on client→backend copy. When client→backend finishes, the function returns, triggering `defer Close()` on both connections. The backend→client goroutine may block indefinitely on a slow backend.

```go
go wp.copyData(clientConn, backendConn, "backend->client")
wp.copyData(backendConn, clientBuf, "client->backend")
return nil  // Goroutine may still be running
```

**Recommendation**: Use `sync.WaitGroup` to wait for both copy directions before closing connections.

---

### C-11: TOCTOU Race in Backend Selection

**Category**: Concurrency
**File**: `internal/router/backend.go:102-133, 284-295` and `internal/router/router.go:81`
**Impact**: `HealthyCount()` is called without holding the BackendPool lock, then `Select()` acquires it separately. A backend can become unhealthy between the two calls.

**Recommendation**: Move the health check inside `Select()` or provide a `SelectHealthy()` method.

---

### C-12: WebSocket Upgrade via Regular Proxy

**Category**: Security / Reliability
**File**: `internal/proxy/proxy.go:67-70`
**Impact**: When a WebSocket upgrade is detected, headers are set but `httputil.ReverseProxy` still handles the request. It doesn't support HTTP hijacking, so WebSocket connections fail or behave incorrectly.

**Recommendation**: Route WebSocket requests to `WebSocketProxy` instead of `httputil.ReverseProxy`.

---

### C-13: Radix Tree Match Returns First Match, Not Longest Prefix

**Category**: Correctness
**File**: `internal/router/radix.go:144-166`
**Impact**: `match()` returns on the first child match. If routes `/api` and `/api/v2` exist, whichever was inserted first wins, not the longest prefix match.

```go
if match := t.match(child, remaining, lastMatch); match != nil {
    return match  // Returns first match, not longest
}
```

**Recommendation**: Explore all matching children and return the longest match by updating `lastMatch` instead of returning early.

---

## HIGH FINDINGS

### H-01: Open Redirect via Host Header

**File**: `cmd/dockrouter/main.go:338-343` and `internal/middleware/redirect.go:15-18`
**Impact**: `r.Host` is client-controlled and used directly in redirect targets. Attacker sends `Host: evil.com` → server redirects to `https://evil.com`.

**Recommendation**: Validate `r.Host` against route table domains.

---

### H-02: Circuit Breaker Unlimited Concurrent Half-Open Requests

**File**: `internal/middleware/circuitbreaker.go:89-90`
**Impact**: In `StateHalfOpen`, every request passes through. Under load, hundreds of requests hit a potentially-down backend simultaneously.

**Recommendation**: Use an atomic counter to limit concurrent half-open probes.

---

### H-03: BuildChain Creates Middleware on Every Request

**File**: `internal/router/route_middleware.go:24-89`
**Impact**: Basic auth user maps, IP filter rules, and CORS configs are rebuilt from slices on every single HTTP request. Massive allocation overhead on the hot path.

**Recommendation**: Cache the built middleware chain per route ID; rebuild only on config change.

---

### H-04: New ReverseProxy Created on Every Request

**File**: `internal/proxy/proxy.go:47-84`
**Impact**: `httputil.NewSingleHostReverseProxy` is called per-request with new closures. At 10K req/s, this creates significant GC pressure.

**Recommendation**: Create the proxy once at `NewProxy` time.

---

### H-05: ACME Client Doesn't Check HTTP Status Codes

**File**: `internal/tls/acme.go:145-220`
**Impact**: All ACME API calls decode response bodies without checking `resp.StatusCode`. Error responses from the ACME server produce confusing JSON decode errors.

**Recommendation**: Check status code first; decode `ACMEError` on non-2xx.

---

### H-06: Health Checker Always Uses HTTP Regardless of Configuration

**File**: `internal/health/checker.go:88`
**Impact**: `TCPCheck` exists but is never called. All health checks use HTTP regardless of backend configuration.

**Recommendation**: Add a `Type` field to `HealthCheck` and dispatch to the appropriate function.

---

### H-07: SSE Hub Double-Close Panic

**File**: `internal/admin/sse.go:76-86, 42-59`
**Impact**: `Stop()` iterates clients and closes channels under lock. `Run()` also closes client channels under lock. If `Stop()` is called while `Run()` processes an unregister, double-close panic occurs.

**Recommendation**: Use `sync.Once` per client or have `Stop()` signal `Run()` to handle all closes.

---

### H-08: SSE Handler Goroutine Leak When Run() Exits

**File**: `internal/admin/sse.go:115-116`
**Impact**: If `Run()` has exited, `Handler()` blocks forever on `h.register <- client` with no receiver. HTTP handler goroutine leaks permanently.

**Recommendation**: Use `select` with `r.Context().Done()` for all channel sends in the handler.

---

### H-09: TLS Store Locks Held During Filesystem I/O

**File**: `internal/tls/store.go:39-59, 75-88`
**Impact**: `Save` and `Load` hold the mutex for the entire duration of file I/O. Concurrent certificate operations are serialized, causing latency spikes.

**Recommendation**: Use per-domain locks or protect only the in-memory index.

---

### H-10: RenewalScheduler.Start Race Condition

**File**: `internal/tls/renewal.go:37-44`
**Impact**: `s.cancel != nil` check and `s.cancel` assignment are not atomic. Concurrent `Start()` calls could both pass the check.

**Recommendation**: Use `sync.Once` for the Start method.

---

### H-11: IPv6 Address Parsing Incorrect

**Files**: `internal/router/router.go:68-70`, `internal/router/table.go:218-224`, `internal/proxy/proxy.go:92-97`
**Impact**: Using `strings.LastIndex(host, ":")` for port stripping is incorrect for IPv6 addresses. Use `net.SplitHostPort`.

---

### H-12: Route Struct Not Thread-Safe for Concurrent Reads

**File**: `internal/router/route.go:10-30`
**Impact**: `MiddlewareConfig` contains slices read during `BuildChain` without synchronization. If a route is updated concurrently, readers see partially-updated slices.

**Recommendation**: Make Route immutable after creation; replace atomically in the Table.

---

### H-13: math/rand Without Seeding for Load Balancing

**File**: `internal/router/backend.go:5, 142`
**Impact**: `selectRandom` uses `rand.Intn()`. In Go <1.20, global source is deterministic. An attacker could predict backend selection.

**Recommendation**: Use `math/rand/v2` or seed with cryptographic randomness.

---

### H-14: Wildcard Host Matching Too Broad

**File**: `internal/router/table.go:227-235`
**Impact**: `*.example.com` matches `evil-example.com` because `evil-example.com` has suffix `.example.com`. Also matches deeply nested subdomains.

```go
suffix := pattern[1:] // .example.com
return strings.HasSuffix(host, suffix) || host == pattern[2:]
```

**Recommendation**: Verify the character before the suffix is a dot.

---

### H-15: Container Assumed Healthy When Running

**File**: `internal/discovery/reconciler.go:196-199`
**Impact**: A container with failing Docker health checks is marked healthy if it's running. Defeats the purpose of health checks.

**Recommendation**: Only mark healthy if `detail.State.Healthy` is true, or if no health check is configured.

---

### H-16: Config Silently Swallows Invalid Environment Variables

**File**: `internal/config/config.go:169-188`
**Impact**: Setting `DR_HTTP_PORT=abc` is silently ignored, app runs on default port. Operator gets zero feedback.

**Recommendation**: Log warnings for invalid env var values.

---

### H-17: FlagSet Registration-Time Overwrite

**File**: `internal/config/flag.go:45-49`
**Impact**: `BoolVar` and `IntVar` overwrite the target at registration time. Works only because the caller passes the current (env-loaded) value as default. A refactoring with literal defaults would silently break env var support.

---

### H-18: Poller Dead Code

**File**: `internal/discovery/poller.go` and `internal/discovery/reconciler.go:56`
**Impact**: `Poller` type created in `NewEngine` but never used. `pollLoop` uses its own hardcoded 30s ticker instead of the configured poll interval.

**Recommendation**: Remove `Poller` or use it properly.

---

### H-19: App Fields Race Condition

**File**: `cmd/dockrouter/main.go:224-276`
**Impact**: Fields written in `start()` and read from HTTP handler goroutines lack memory ordering guarantees per the Go memory model.

**Recommendation**: Ensure all App fields are set before launching goroutines.

---

### H-20: GetContainers Shallow Copy Leaks Internal State

**File**: `internal/discovery/reconciler.go:365-375`
**Impact**: `cp := *info` is a shallow copy. `Config *RouteConfig` and `Labels map[string]string` are shared pointers. Concurrent modification races with admin API reads.

**Recommendation**: Deep copy pointer fields.

---

### H-21: Metrics Active Request Gauge Leaks on Panic

**File**: `internal/middleware/metrics.go:29, 36`
**Impact**: `IncGauge` is not paired with a deferred `DecGauge`. If a downstream handler panics and Recovery is not outermost, the gauge leaks.

**Recommendation**: Use `defer collector.DecGauge(...)`.

---

## MEDIUM FINDINGS

### M-01: ACME Staging Ignored for Non-Let's Encrypt Providers
**File**: `internal/config/defaults.go:60-70`
`ACMEStaging=true` always returns Let's Encrypt staging regardless of `ACMEProvider`.

### M-02: Rate Limiter Off-by-One on First Request
**File**: `internal/middleware/ratelimit.go:90-91`
New buckets start at `maxSize-1` tokens instead of `maxSize`.

### M-03: Rate Limiter Unbounded Memory Growth Under DDoS
**File**: `internal/middleware/ratelimit.go:13-21`
No upper bound on bucket count. Unique IP DDoS fills memory before cleanup runs.

### M-04: Compress Middleware No Content-Type Filtering
**File**: `internal/middleware/compress.go:59-61`
Gzip initialized for all responses regardless of Content-Type. Wastes CPU on incompressible data.

### M-05: gzipResponseWriter Missing http.Flusher
**File**: `internal/middleware/compress.go:31-34`
Breaks SSE and streaming responses that call `Flush()`.

### M-06: CORS Wildcard + Credentials Allowed
**File**: `internal/middleware/cors.go:29-31`
No validation that `Origins=["*"]` and `Credentials=true` are mutually exclusive.

### M-07: IPFilter Trusts Leftmost XFF IP
**File**: `internal/middleware/ipfilter.go:121-130`
Should walk XFF from right to left, stopping at first untrusted IP.

### M-08: AccessLog Log Injection via URL Path
**File**: `internal/middleware/accesslog.go:29-35`
`r.URL.Path` containing `\n`/`\r` injects fake log lines.

### M-09: ResponseWriter Wrappers Don't Propagate Interfaces
**Files**: Multiple middleware files
Wrapped writers don't implement `http.Flusher`, `http.Hijacker`, `http.Push`. Breaks WebSocket, HTTP/2 push, streaming.

### M-10: StripPrefix No Path Normalization
**File**: `internal/middleware/pathmod.go:17`
After stripping, remaining path could contain `//` or `..` segments.

### M-11: AddPrefix Doesn't Update RawPath
**File**: `internal/middleware/pathmod.go:35`
Stale `RawPath` causes routing inconsistencies for percent-encoded URLs.

### M-12: Docker API Path Injection
**File**: `internal/discovery/docker.go:261-262`
Container ID interpolated into API path without validation or URL encoding.

### M-13: Docker Response Body No Size Limit
**File**: `internal/discovery/docker.go:85`
`io.ReadAll(resp.Body)` reads unbounded response from Docker socket. OOM risk.

### M-14: Docker Error Body May Leak in Logs
**File**: `internal/discovery/docker.go:80-81`
Full Docker API error body embedded in error messages.

### M-15: Variable Shadowing of `url` Package
**File**: `internal/discovery/docker.go:59, 102`
Local variable `url` shadows `net/url` import.

### M-16: Poll Errors Silently Swallowed
**File**: `internal/discovery/poller.go:49-53`
Docker daemon unreachability produces no feedback.

### M-17: Configured Poll Interval Ignored
**File**: `internal/discovery/reconciler.go:349`
`pollLoop` uses hardcoded 30s instead of `Config.PollInterval`.

### M-18: Health Check Zero Threshold
**File**: `internal/health/checker.go:119`
With `Threshold=0`, first failure immediately marks unhealthy.

### M-19: HTTPCheck Target URL Not Validated
**File**: `internal/health/http.go:29`
Target concatenated directly into URL without sanitization.

### M-20: Admin API No Method Validation
**File**: `internal/admin/api.go:24-33`
All endpoints accept any HTTP method (GET, POST, DELETE, etc.).

### M-21: Admin Auth Empty Credentials Bypass
**File**: `internal/admin/auth.go:27-30`
When username/password are both empty, auth is silently disabled. No warning logged.

### M-22: SSE Events Silently Dropped
**File**: `internal/admin/sse.go:89-95`
Broadcast channel full → event dropped with no logging or metric.

### M-23: TLS MinVersion Allows TLS 1.2
**File**: `internal/tls/manager.go:413-424`
`MinVersion: tls.VersionTLS12` allows TLS 1.2 connections. Should prefer TLS 1.3.

### M-24: Manager.LoadAccountKey No Nil Check
**File**: `internal/tls/manager.go:498`
Accesses `m.acme.privateKey` without checking if `m.acme` is nil.

### M-25: Manager.SaveAccountKey Ignores MkdirAll Error
**File**: `internal/tls/manager.go:480`
`os.MkdirAll` return value discarded.

### M-26: Changed() No Nil Check on Config
**File**: `internal/discovery/reconciler.go:206-211`
Panics if `ContainerInfo.Config` is nil.

### M-27: watchEvents Sets running=false on Disconnect
**File**: `internal/discovery/reconciler.go:215-219`
`running` set false on stream disconnect, not just context cancellation. Inconsistent with pollLoop.

### M-28: Logger Ignores json.Marshal and Write Errors
**File**: `internal/log/logger.go:118`
Both `json.Marshal` and `l.w.Write` errors silently discarded.

### M-29: Logger.With Captures Level by Value
**File**: `internal/log/logger.go:170`
Dynamic level changes on parent don't propagate to child loggers.

### M-30: Prometheus sanitizeName Incomplete
**File**: `internal/metrics/prometheus.go:44-49`
Only replaces `-` and `.`, not spaces, parentheses, slashes, etc.

### M-31: Metrics Collector Write Lock for Read-Mostly Operations
**File**: `internal/metrics/collector.go:56-65`
Full `Lock()` for get-or-create pattern. Should use RLock first.

### M-32: Admin Bind Empty String Bypass
**File**: `internal/config/validate.go:38`
Empty `AdminBind` skips validation and defaults to binding all interfaces.

### M-33: parseFlags Calls os.Exit
**File**: `internal/config/config.go:116-128`
`os.Exit(0)` on --help/--version prevents testing and bypasses deferred cleanup.

### M-34: No Request Timeout Middleware
**Files**: Cross-cutting
No global request timeout. Slow backends hold goroutines indefinitely.

---

## LOW FINDINGS

### L-01: Custom `intToStr` Replaces `strconv.Itoa`
**Files**: `internal/router/router.go:237-255`, `internal/discovery/reconciler.go:415-433`, `internal/middleware/ratelimit.go:137`
O(n^2) prepend-based implementation. Replace with `strconv.Itoa`.

### L-02: Duplicated `buildErrorPage`
**Files**: `internal/router/router.go:202-235`, `internal/proxy/proxy.go:178-210`
Two nearly identical implementations. Extract to shared utility.

### L-03: Predictable Request ID Fallback
**File**: `internal/middleware/requestid.go:30-34`
Fallback generates deterministic `000102030405060708090a0b0c0d0e0f`.

### L-04: X-XSS-Protection Header Deprecated
**File**: `internal/middleware/security.go:11`
Should use CSP instead, or set to `0`.

### L-05: Missing Content-Security-Policy Header
**File**: `internal/middleware/security.go:8-18`

### L-06: Dockerfile HEALTHCHECK Syntax Error
**File**: `Dockerfile:52-53`
`|| exit 1` after JSON-formatted CMD is invalid and silently ignored.

### L-07: ACME Email Validation Only Checks for @
**File**: `internal/config/validate.go:92-93`

### L-08: Multiple Logger Interface Definitions
**Files**: `internal/middleware/accesslog.go:11-14`, `internal/tls/renewal.go:20-25`
Incompatible interfaces across packages. Should use shared definition.

### L-09: EventTimestamp Ignores Nanosecond Precision
**File**: `internal/discovery/events.go:93-95`

### L-10: ForceAttemptHTTP2: false is Default
**File**: `internal/proxy/transport.go:33`
Explicit setting of zero value is unnecessary.

### L-11: extractName Misleading for-range
**File**: `internal/discovery/reconciler.go:391-396`
Uses `for range` as nil check; only ever iterates once.

### L-12: RawLabels Stores Reference to External Map
**File**: `internal/discovery/labels.go:156`
No defensive copy; caller modifications visible in RouteConfig.

### L-13: Logger.Fields json:"-" Tag Misleading
**File**: `internal/log/logger.go:67`
Custom MarshalJSON flattens Fields despite `json:"-"` tag.

### L-14: AccessLogEntry.DurationMs Precision
**File**: `internal/log/access.go:34`
Microseconds / 1000 could use cleaner expression.

### L-15: Docker Client Not Closed on Shutdown
**File**: `cmd/dockrouter/main.go:170-186`
No `Close()` method or cleanup path.

### L-16: HTTP Servers Missing MaxHeaderBytes
**File**: `cmd/dockrouter/main.go:224-248`

### L-17: Health Check Client Missing Transport Limits
**File**: `cmd/dockrouter/main.go:689`
Default transport has unlimited idle connections.

### L-18: Wildcard Matching O(n) Per Pattern
**File**: `internal/router/table.go:92-98`
Linear scan over wildcard patterns on every request.

---

## Positive Observations

1. **Zero external dependencies** — eliminates supply chain attack surface
2. **Proper `sync.RWMutex` usage** — read-heavy structures use `RLock` correctly
3. **Constant-time auth comparison** — `crypto/subtle.ConstantTimeCompare` used for all auth
4. **HTML escaping in error pages** — `html.EscapeString` prevents XSS
5. **Structured JSON logging** — with level and field support
6. **`http.MaxBytesReader`** — proper body size limiting
7. **ACME account key persistence** — prevents unnecessary account recreation
8. **Graceful shutdown** — SIGINT/SIGTERM with 30s timeout
9. **Challenge deduplication** — `sync.Map` with `LoadOrStore` for concurrent cert provisioning
10. **Container copy on read** — `GetContainers` returns copies (though shallow)

---

## Recommended Action Plan

### Phase 1 — Critical Fixes (1-2 weeks)

| Priority | Issue | Effort |
|----------|-------|--------|
| P0 | Fix retry/failover with buffered response writer | 2-3 days |
| P0 | Fix on-demand TLS with self-signed fallback | 1-2 days |
| P0 | Add ACME nonce mutex | 0.5 day |
| P0 | Fix WebSocket response validation and truncation | 1 day |
| P0 | Fix reconciler race condition (consistent locking) | 1 day |
| P0 | Fix Docker event stream goroutine leak | 1 day |
| P0 | Fix XFF injection (overwrite instead of append) | 0.5 day |
| P0 | Remove radix tree sync.Pool | 0.5 day |
| P0 | Fix WebSocket goroutine leak (WaitGroup) | 0.5 day |
| P0 | Fix radix tree longest-prefix match | 0.5 day |

### Phase 2 — High Fixes (2-3 weeks)

| Priority | Issue | Effort |
|----------|-------|--------|
| P1 | Open redirect fix (Host validation) | 0.5 day |
| P1 | Cache middleware chains per route | 1 day |
| P1 | Reuse ReverseProxy instance | 0.5 day |
| P1 | ACME status code checking | 1 day |
| P1 | Health check type dispatch | 0.5 day |
| P1 | SSE hub race fixes | 1 day |
| P1 | IPv6 parsing (use net.SplitHostPort) | 0.5 day |
| P1 | Deep copy in GetContainers | 0.5 day |
| P1 | Route struct immutability | 1 day |
| P1 | Circuit breaker half-open limiting | 0.5 day |

### Phase 3 — Medium Fixes (3-4 weeks)

All M-01 through M-34 items. Estimated 5-7 days total effort.

### Phase 4 — Low Fixes / Tech Debt (Ongoing)

Replace `intToStr`, extract shared utilities, improve documentation, add missing tests.

---

## Estimated Remediation Effort

| Phase | Duration | Issues |
|-------|----------|--------|
| Phase 1 (Critical) | 1-2 weeks | 13 CRITICAL |
| Phase 2 (High) | 2-3 weeks | 21 HIGH |
| Phase 3 (Medium) | 3-4 weeks | 34 MEDIUM |
| Phase 4 (Low) | Ongoing | 18 LOW |

**Total estimated effort**: 6-10 weeks for full remediation, with critical fixes achievable in 1-2 weeks.
