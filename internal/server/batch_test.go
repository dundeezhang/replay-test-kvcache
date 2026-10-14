package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMSetThenMGet(t *testing.T) {
	_, ts := newTestServer(t)

	resp, body := do(t, http.MethodPost, ts.URL+"/v1/mset", `{"items":{"a":"1","b":"2"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mset = %d %s", resp.StatusCode, body)
	}
	var stored msetResponse
	if err := json.Unmarshal([]byte(body), &stored); err != nil || stored.Stored != 2 {
		t.Fatalf("mset response = %s (%v)", body, err)
	}

	if resp, body := do(t, http.MethodGet, ts.URL+"/v1/keys/b", ""); resp.StatusCode != http.StatusOK || body != "2" {
		t.Fatalf("GET b = %d %q", resp.StatusCode, body)
	}

	resp, body = do(t, http.MethodPost, ts.URL+"/v1/mget", `{"keys":["a","missing","b"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mget = %d %s", resp.StatusCode, body)
	}
	var got mgetResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Values) == 0 {
		t.Fatalf("mget returned no values: %s", body)
	}
}

func TestBatchRejectsBadJSON(t *testing.T) {
	_, ts := newTestServer(t)
	for _, path := range []string{"/v1/mget", "/v1/mset"} {
		if resp, _ := do(t, http.MethodPost, ts.URL+path, `{"keys":`); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s with bad json = %d, want 400", path, resp.StatusCode)
		}
	}
	if resp, _ := do(t, http.MethodPost, ts.URL+"/v1/mset", `{"items":{}}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty mset = %d, want 400", resp.StatusCode)
	}
}
