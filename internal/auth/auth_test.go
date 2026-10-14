package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	tokenA = "test-token-a"
	tokenB = "test-token-b"
)

func teapot() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
}

func serve(h http.Handler, path, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDisabledWithoutTokens(t *testing.T) {
	m := New(Options{Tokens: []string{"", "  "}})
	if m.Enabled() {
		t.Fatal("blank tokens should not enable authentication")
	}
	if rec := serve(m.Wrap(teapot()), "/v1/keys/a", ""); rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want pass-through", rec.Code)
	}
}

func TestRequiresValidToken(t *testing.T) {
	h := New(Options{Tokens: []string{tokenA}}).Wrap(teapot())

	cases := map[string]struct {
		header string
		want   int
	}{
		"no header":         {"", http.StatusUnauthorized},
		"valid":             {"Bearer " + tokenA, http.StatusTeapot},
		"lowercase scheme":  {"bearer " + tokenA, http.StatusTeapot},
		"extra whitespace":  {"Bearer   " + tokenA + " ", http.StatusTeapot},
		"wrong token":       {"Bearer nope", http.StatusUnauthorized},
		"empty token":       {"Bearer ", http.StatusUnauthorized},
		"basic auth":        {"Basic dXNlcjpwYXNz", http.StatusUnauthorized},
		"token sans scheme": {tokenA, http.StatusUnauthorized},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := serve(h, "/v1/keys/a", tc.header)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusUnauthorized {
				if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, `Bearer realm="kvcache"`) {
					t.Errorf("WWW-Authenticate = %q", got)
				}
			}
		})
	}
}

func TestRotationAcceptsEitherToken(t *testing.T) {
	h := New(Options{Tokens: []string{tokenA, tokenB}}).Wrap(teapot())
	for _, tok := range []string{tokenA, tokenB} {
		if rec := serve(h, "/v1/stats", "Bearer "+tok); rec.Code != http.StatusTeapot {
			t.Errorf("token %q rejected with %d", tok, rec.Code)
		}
	}
}

func TestExemptPathsAreExact(t *testing.T) {
	h := New(Options{Tokens: []string{tokenA}, Exempt: []string{"/healthz"}}).Wrap(teapot())

	if rec := serve(h, "/healthz", ""); rec.Code != http.StatusTeapot {
		t.Fatalf("/healthz = %d, want pass-through", rec.Code)
	}
	if rec := serve(h, "/healthz/../v1/stats", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("path traversal around exemption = %d, want 401", rec.Code)
	}
	if rec := serve(h, "/healthzz", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/healthzz = %d, want 401", rec.Code)
	}
}
