package main

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const syntheticRealCode = "AMD1-0000-0000-0000-0000-0000-0000-0000-0000"
const syntheticDisplayCode = "AVD1-3A69-EA03-74CC-555A-6EC1-03FC-DF39-B012"

func TestDeviceCodeFilteringPreservesJSONAndLargeCounters(t *testing.T) {
	for _, raw := range []string{
		`{"device_id":"` + syntheticRealCode + `","count":9007199254740993,"activated":true}`,
		`{"detail":"failed for \u0041MD1-0000-0000-0000-0000-0000-0000-0000-0000","count":9007199254740993,"activated":true}`,
	} {
		filtered := string(displayDeviceJSON([]byte(raw)))
		if strings.Contains(filtered, "AMD1-") || !strings.Contains(filtered, syntheticDisplayCode) ||
			!strings.Contains(filtered, "9007199254740993") || !strings.Contains(filtered, `"activated":true`) {
			t.Fatal("device filtering changed unrelated fields or exposed the real code")
		}
	}
	if strings.Contains(displayDeviceText("AMD1-UNAVAILABLE"), "AMD1-") {
		t.Fatal("malformed code exposed")
	}
}

func TestApplicationJSONAndLogsUseDisplayCodes(t *testing.T) {
	w := httptest.NewRecorder()
	jsonOut(w, 200, map[string]any{"snapshot": map[string]any{"device_id": syntheticRealCode}})
	if strings.Contains(w.Body.String(), syntheticRealCode) || !strings.Contains(w.Body.String(), syntheticDisplayCode) {
		t.Fatal("application JSON exposed a real code")
	}
	line := redact("license check for " + syntheticRealCode)
	if strings.Contains(line, syntheticRealCode) || !strings.Contains(line, syntheticDisplayCode) {
		t.Fatal("log redaction exposed a real code")
	}
}

func TestEngineResponseFilteringPreservesStatusAndLength(t *testing.T) {
	for _, status := range []int{200, 403} {
		response := &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": {"application/json"}, "Etag": {"private-value"}},
			Body:       io.NopCloser(strings.NewReader(`{"device_id":"` + syntheticRealCode + `"}`)),
		}
		if err := displayDeviceResponse(response); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != status || response.ContentLength != int64(len(body)) ||
			strings.Contains(string(body), syntheticRealCode) || !strings.Contains(string(body), syntheticDisplayCode) ||
			response.Header.Get("ETag") != "" {
			t.Fatal("engine response did not preserve its contract safely")
		}
	}
}

type fragmentedDeviceStream struct {
	reader io.Reader
	closed bool
}

func (s *fragmentedDeviceStream) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return s.reader.Read(buffer)
}
func (s *fragmentedDeviceStream) Close() error { s.closed = true; return nil }

func TestEventStreamFiltersFragmentedCodesAndClosesSource(t *testing.T) {
	input := "event: license\ndata: {\"device_id\":\"" + syntheticRealCode + "\"}\n\n: keepalive\n\n"
	source := &fragmentedDeviceStream{reader: strings.NewReader(input)}
	stream := &deviceDisplayStream{source: source, reader: bufio.NewReader(source)}
	body, err := io.ReadAll(stream)
	if err != nil || strings.Contains(string(body), syntheticRealCode) ||
		!strings.Contains(string(body), syntheticDisplayCode) || !strings.Contains(string(body), ": keepalive\n\n") {
		t.Fatal("fragmented event filtering failed")
	}
	stream.Close()
	if !source.closed {
		t.Fatal("event stream source was not released")
	}
}
