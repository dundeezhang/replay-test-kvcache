package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func getTTL(t *testing.T, url string) (int, ttlResponse) {
	t.Helper()
	resp, body := do(t, http.MethodGet, url, "")
	var out ttlResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("decode %q: %v", body, err)
		}
	}
	return resp.StatusCode, out
}

func TestSetWithTTLQueryParam(t *testing.T) {
	_, ts := newTestServer(t)

	resp, _ := do(t, http.MethodPut, ts.URL+"/v1/keys/session?ttl=60", "abc")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT = %d, want 204", resp.StatusCode)
	}

	code, out := getTTL(t, ts.URL+"/v1/keys/session/ttl")
	if code != http.StatusOK {
		t.Fatalf("GET ttl = %d", code)
	}
	if out.TTLSeconds < 59 || out.TTLSeconds > 60 {
		t.Fatalf("ttl_seconds = %d, want ~60", out.TTLSeconds)
	}
	if out.ExpiresAt == nil {
		t.Fatal("expires_at missing for an expiring key")
	}
}

func TestTTLForPersistentKey(t *testing.T) {
	_, ts := newTestServer(t)
	do(t, http.MethodPut, ts.URL+"/v1/keys/forever", "x")

	code, out := getTTL(t, ts.URL+"/v1/keys/forever/ttl")
	if code != http.StatusOK || out.TTLSeconds != -1 || out.ExpiresAt != nil {
		t.Fatalf("GET ttl = %d %+v", code, out)
	}
}

func TestExpireAndPersistEndpoints(t *testing.T) {
	_, ts := newTestServer(t)
	keyURL := ts.URL + "/v1/keys/k"
	do(t, http.MethodPut, keyURL, "v")

	resp, _ := do(t, http.MethodPut, keyURL+"/ttl", `{"ttl_seconds": 30}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT ttl = %d", resp.StatusCode)
	}
	if _, out := getTTL(t, keyURL+"/ttl"); out.TTLSeconds != 30 {
		t.Fatalf("ttl_seconds after expire = %d", out.TTLSeconds)
	}

	resp, _ = do(t, http.MethodDelete, keyURL+"/ttl", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE ttl = %d", resp.StatusCode)
	}
	if _, out := getTTL(t, keyURL+"/ttl"); out.TTLSeconds != -1 {
		t.Fatalf("ttl_seconds after persist = %d", out.TTLSeconds)
	}
}

func TestExpireRejectsBadInput(t *testing.T) {
	_, ts := newTestServer(t)
	keyURL := ts.URL + "/v1/keys/k"
	do(t, http.MethodPut, keyURL, "v")

	cases := map[string]struct {
		url  string
		body string
		want int
	}{
		"zero ttl":      {keyURL + "/ttl", `{"ttl_seconds": 0}`, http.StatusBadRequest},
		"huge ttl":      {keyURL + "/ttl", `{"ttl_seconds": 999999999999}`, http.StatusBadRequest},
		"unknown field": {keyURL + "/ttl", `{"ttl": 5}`, http.StatusBadRequest},
		"not json":      {keyURL + "/ttl", `soon`, http.StatusBadRequest},
		"missing key":   {ts.URL + "/v1/keys/nope/ttl", `{"ttl_seconds": 5}`, http.StatusNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resp, body := do(t, http.MethodPut, tc.url, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d (%s)", resp.StatusCode, tc.want, body)
			}
		})
	}
}

func TestNegativeTTLQueryParamRejected(t *testing.T) {
	_, ts := newTestServer(t)
	resp, _ := do(t, http.MethodPut, ts.URL+"/v1/keys/k?ttl=-5", "v")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT with negative ttl = %d, want 400", resp.StatusCode)
	}
}
