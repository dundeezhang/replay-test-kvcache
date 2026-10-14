// Package snapshot persists store contents to disk and restores them at
// startup.
//
// A snapshot file has the following layout; all integers are big-endian:
//
//	magic    [4]byte   "KVS1"
//	created  uint64    unix nanoseconds
//	records  ...       repeated until a record with keyLen == 0
//	count    uint64    number of records, cross-checked on read
//	crc      uint32    CRC-32C of every preceding byte
//
// Each record is:
//
//	keyLen   uint32
//	key      [keyLen]byte
//	version  uint64
//	valLen   uint32
//	value    [valLen]byte
//
// Empty keys are rejected by the store, so a zero keyLen can safely mark
// the end of the record stream. This lets the encoder stream shards to
// disk without knowing the total record count up front.
package snapshot

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"time"
)

var magic = [4]byte{'K', 'V', 'S', '1'}

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

var (
	// ErrBadMagic means the input is not a kvcache snapshot.
	ErrBadMagic = errors.New("snapshot: not a snapshot file")
	// ErrChecksum means the input is corrupt.
	ErrChecksum = errors.New("snapshot: checksum mismatch")
)

// Record is one key/value pair in a snapshot.
type Record struct {
	Key     string
	Value   []byte
	Version uint64
}

// Encoder writes a snapshot stream. Errors are sticky: after the first
// failed write every later call returns the same error.
type Encoder struct {
	bw    *bufio.Writer
	crc   hash.Hash32
	count uint64
	buf   [8]byte
	err   error
}

// NewEncoder writes the snapshot header to w and returns an Encoder.
func NewEncoder(w io.Writer, created time.Time) *Encoder {
	e := &Encoder{
		bw:  bufio.NewWriterSize(w, 64<<10),
		crc: crc32.New(castagnoli),
	}
	e.write(magic[:])
	e.putUint64(uint64(created.UnixNano()))
	return e
}

// Encode appends one record.
func (e *Encoder) Encode(r Record) error {
	if r.Key == "" {
		return errors.New("snapshot: cannot encode an empty key")
	}
	e.putUint32(uint32(len(r.Key)))
	e.write([]byte(r.Key))
	e.putUint64(r.Version)
	e.putUint32(uint32(len(r.Value)))
	e.write(r.Value)
	if e.err == nil {
		e.count++
	}
	return e.err
}

// Count returns the number of records encoded so far.
func (e *Encoder) Count() uint64 { return e.count }

// Close writes the trailer and flushes buffered data. It does not close
// the underlying writer.
func (e *Encoder) Close() error {
	e.putUint32(0)
	e.putUint64(e.count)
	if e.err != nil {
		return e.err
	}
	binary.BigEndian.PutUint32(e.buf[:4], e.crc.Sum32())
	if _, err := e.bw.Write(e.buf[:4]); err != nil {
		return err
	}
	return e.bw.Flush()
}

func (e *Encoder) write(p []byte) {
	if e.err != nil {
		return
	}
	e.crc.Write(p)
	_, e.err = e.bw.Write(p)
}

func (e *Encoder) putUint32(v uint32) {
	binary.BigEndian.PutUint32(e.buf[:4], v)
	e.write(e.buf[:4])
}

func (e *Encoder) putUint64(v uint64) {
	binary.BigEndian.PutUint64(e.buf[:8], v)
	e.write(e.buf[:8])
}

// Decoder reads a snapshot stream produced by Encoder.
type Decoder struct {
	br      *bufio.Reader
	crc     hash.Hash32
	created time.Time
	count   uint64
	done    bool
	buf     [8]byte
}

// NewDecoder reads and checks the snapshot header.
func NewDecoder(r io.Reader) (*Decoder, error) {
	d := &Decoder{
		br:  bufio.NewReaderSize(r, 64<<10),
		crc: crc32.New(castagnoli),
	}
	var m [4]byte
	if err := d.read(m[:]); err != nil {
		return nil, err
	}
	if m != magic {
		return nil, ErrBadMagic
	}
	ns, err := d.uint64()
	if err != nil {
		return nil, err
	}
	d.created = time.Unix(0, int64(ns)).UTC()
	return d, nil
}

// Created returns the time recorded in the snapshot header.
func (d *Decoder) Created() time.Time { return d.created }

// Next returns the next record. After the last record it verifies the
// trailer and returns io.EOF; any other error means the snapshot is
// truncated or corrupt.
func (d *Decoder) Next() (Record, error) {
	if d.done {
		return Record{}, io.EOF
	}

	keyLen, err := d.uint32()
	if err != nil {
		return Record{}, err
	}
	if keyLen == 0 {
		return Record{}, d.finish()
	}
	key := make([]byte, keyLen)
	if err := d.read(key); err != nil {
		return Record{}, err
	}
	version, err := d.uint64()
	if err != nil {
		return Record{}, err
	}
	valLen, err := d.uint32()
	if err != nil {
		return Record{}, err
	}
	value := make([]byte, valLen)
	if err := d.read(value); err != nil {
		return Record{}, err
	}

	d.count++
	return Record{Key: string(key), Value: value, Version: version}, nil
}

func (d *Decoder) finish() error {
	d.done = true
	n, err := d.uint64()
	if err != nil {
		return err
	}
	if n != d.count {
		return fmt.Errorf("%w: trailer counts %d records, read %d", ErrChecksum, n, d.count)
	}

	want := d.crc.Sum32()
	var b [4]byte
	if _, err := io.ReadFull(d.br, b[:]); err != nil {
		return io.ErrUnexpectedEOF
	}
	if binary.BigEndian.Uint32(b[:]) != want {
		return ErrChecksum
	}
	return io.EOF
}

func (d *Decoder) read(p []byte) error {
	if _, err := io.ReadFull(d.br, p); err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	d.crc.Write(p)
	return nil
}

func (d *Decoder) uint32() (uint32, error) {
	if err := d.read(d.buf[:4]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(d.buf[:4]), nil
}

func (d *Decoder) uint64() (uint64, error) {
	if err := d.read(d.buf[:8]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(d.buf[:8]), nil
}
