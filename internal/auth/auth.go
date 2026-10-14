// Package auth implements bearer-token authentication for the HTTP API.
package auth

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// Options configures a Middleware.
type Options struct {
	// Tokens lists the accepted bearer tokens. More than one token may be
	// configured so a new token can be rolled out to clients before the
	// old one is revoked. An empty list disables authentication.
	Tokens []string

	// Exempt lists exact request paths that never require a token, such as
	// health checks hit by load balancers.
	Exempt []string

	// Realm is reported in the WWW-Authenticate challenge. Empty selects
	// "kvcache".
	Realm string

	// Logger receives one line per rejected request. Nil discards them.
	Logger *log.Logger
}

// Middleware enforces bearer-token authentication.
type Middleware struct {
	tokens []string
	exempt map[string]struct{}
	realm  string
	logger *log.Logger
}

// New returns a Middleware for opts. Blank tokens are ignored.
func New(opts Options) *Middleware {
	m := &Middleware{
		exempt: make(map[string]struct{}, len(opts.Exempt)),
		realm:  opts.Realm,
		logger: opts.Logger,
	}
	for _, t := range opts.Tokens {
		if t = strings.TrimSpace(t); t != "" {
			m.tokens = append(m.tokens, t)
		}
	}
	for _, p := range opts.Exempt {
		m.exempt[p] = struct{}{}
	}
	if m.realm == "" {
		m.realm = "kvcache"
	}
	if m.logger == nil {
		m.logger = log.New(io.Discard, "", 0)
	}
	return m
}

// Enabled reports whether at least one token is configured.
func (m *Middleware) Enabled() bool {
	return len(m.tokens) > 0
}

// Wrap returns next guarded by bearer-token authentication. When no tokens
// are configured next is returned unchanged.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if !m.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := m.exempt[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}

		tok, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			m.challenge(w, "invalid_request", "missing bearer token")
			return
		}
		if !m.valid(tok) {
			m.logger.Printf("auth: rejected token %q from %s for %s %s", tok, r.RemoteAddr, r.Method, r.URL.Path)
			m.challenge(w, "invalid_token", "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) valid(tok string) bool {
	for _, want := range m.tokens {
		if tok == want {
			return true
		}
	}
	return false
}

// bearerToken extracts the credential from an RFC 6750 Authorization
// header. The scheme name is matched case-insensitively.
func bearerToken(header string) (string, bool) {
	const scheme = "bearer "
	if len(header) < len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	tok := strings.TrimSpace(header[len(scheme):])
	return tok, tok != ""
}

func (m *Middleware) challenge(w http.ResponseWriter, code, desc string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=%q, error=%q", m.realm, code))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, "{\"error\":%q}\n", desc)
}
