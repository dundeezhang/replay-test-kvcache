package snapshot

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

var testCreated = time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

func encode(t *testing.T, recs []Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := NewEncoder(&buf, testCreated)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			t.Fatalf("Encode(%q): %v", r.Key, err)
		}
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

func decodeAll(data []byte) ([]Record, time.Time, error) {
	dec, err := NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, time.Time{}, err
	}
	var out []Record
	for {
		r, err := dec.Next()
		if errors.Is(err, io.EOF) {
			return out, dec.Created(), nil
		}
		if err != nil {
			return out, dec.Created(), err
		}
		out = append(out, r)
	}
}

func TestRoundTrip(t *testing.T) {
	in := []Record{
		{Key: "a", Value: []byte("1"), Version: 1},
		{Key: "user:42", Value: []byte(`{"name":"ada"}`), Version: 7},
		{Key: "empty", Value: []byte{}, Version: 3},
		{Key: "binary", Value: []byte{0, 1, 2, 255}, Version: 1},
	}
	out, created, err := decodeAll(encode(t, in))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !created.Equal(testCreated) {
		t.Errorf("created = %s, want %s", created, testCreated)
	}
	if len(out) != len(in) {
		t.Fatalf("got %d records, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].Key != in[i].Key || !bytes.Equal(out[i].Value, in[i].Value) || out[i].Version != in[i].Version {
			t.Errorf("record %d = %+v, want %+v", i, out[i], in[i])
		}
	}
}

func TestDetectsCorruptValue(t *testing.T) {
	data := encode(t, []Record{{Key: "k", Value: []byte("hello"), Version: 1}})

	// header (4 magic + 8 created) + keyLen(4) + key(1) + version(8) + valLen(4)
	valueOffset := 12 + 4 + 1 + 8 + 4
	data[valueOffset] ^= 0xff

	if _, _, err := decodeAll(data); !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
}

func TestDetectsTruncation(t *testing.T) {
	data := encode(t, []Record{
		{Key: "a", Value: []byte("alpha"), Version: 1},
		{Key: "b", Value: []byte("bravo"), Version: 2},
	})
	for cut := 1; cut < len(data); cut++ {
		if _, _, err := decodeAll(data[:cut]); err == nil {
			t.Fatalf("decoding %d of %d bytes succeeded", cut, len(data))
		}
	}
}

func TestBadMagic(t *testing.T) {
	data := encode(t, nil)
	data[0] = 'X'
	if _, err := NewDecoder(bytes.NewReader(data)); !errors.Is(err, ErrBadMagic) {
		t.Fatalf("err = %v, want ErrBadMagic", err)
	}
}
