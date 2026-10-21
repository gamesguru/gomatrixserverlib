package fclient

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// endTrackingReader notes whether the reader was read to its end.
type endTrackingReader struct {
	r     io.Reader
	atEOF bool
}

func (e *endTrackingReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if errors.Is(err, io.EOF) {
		e.atEOF = true
	}
	return n, err
}

func TestDecodeJSONBodyReadsToTheEnd(t *testing.T) {
	// The value is followed by what a decoder would leave unread.
	body := &endTrackingReader{r: strings.NewReader(`{"a":1}` + "\n\n")}
	var v struct {
		A int `json:"a"`
	}
	if err := decodeJSONBody(body, &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 1 {
		t.Fatalf("decoded %+v, want a=1", v)
	}
	if !body.atEOF {
		t.Fatal("the body was not read to its end")
	}
}

func TestDecodeJSONBodyReportsABadValue(t *testing.T) {
	var v struct{}
	if err := decodeJSONBody(strings.NewReader(`{"a":`), &v); err == nil {
		t.Fatal("expected an error for a truncated value")
	}
}

func TestDecodeJSONBodyIgnoresWhatFollowsTheValue(t *testing.T) {
	var v struct {
		A int `json:"a"`
	}
	if err := decodeJSONBody(strings.NewReader(`{"a":2} trailing`), &v); err != nil {
		t.Fatalf("a decoder ignores what follows the value: %v", err)
	}
	if v.A != 2 {
		t.Fatalf("decoded %+v, want a=2", v)
	}
}
