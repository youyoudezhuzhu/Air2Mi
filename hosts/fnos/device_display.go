package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var realDeviceCodePattern = regexp.MustCompile(`(?i)\bAMD1-[A-Z0-9-]+\b`)

func displayDeviceCode(code string) string {
	hexCode := strings.ReplaceAll(strings.TrimPrefix(strings.ToUpper(code), "AMD1-"), "-", "")
	data, err := hex.DecodeString(hexCode)
	if err != nil || len(data) != 16 {
		return "[device-code-unavailable]"
	}
	mask := sha256.Sum256([]byte("air2mi.device.display.v1"))
	for i := range data {
		data[i] ^= mask[i]
	}
	encoded := strings.ToUpper(hex.EncodeToString(data))
	groups := make([]string, 0, 8)
	for i := 0; i < len(encoded); i += 4 {
		groups = append(groups, encoded[i:i+4])
	}
	return "AVD1-" + strings.Join(groups, "-")
}

func displayDeviceText(text string) string {
	return realDeviceCodePattern.ReplaceAllStringFunc(text, displayDeviceCode)
}

func displayDeviceJSON(raw []byte) []byte {
	// Normalize JSON escapes before filtering, including codes inside error text.
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if json.Valid(raw) && decoder.Decode(&value) == nil {
		if normalized, err := json.Marshal(value); err == nil {
			raw = normalized
		}
	}
	return []byte(displayDeviceText(string(raw)))
}

// Preserve live event delivery while keeping codes split across network reads
// together. SSE fields are line-delimited and device codes contain no newlines.
type deviceDisplayStream struct {
	source  io.ReadCloser
	reader  *bufio.Reader
	pending []byte
	err     error
}

func (s *deviceDisplayStream) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if len(s.pending) == 0 && s.err == nil {
		line, err := s.reader.ReadBytes('\n')
		if bytes.HasPrefix(line, []byte("data:")) {
			line = append([]byte("data:"), displayDeviceJSON(bytes.TrimSuffix(line[5:], []byte("\n")))...)
			line = append(line, '\n')
		} else {
			line = []byte(displayDeviceText(string(line)))
		}
		s.pending, s.err = line, err
	}
	n := copy(buffer, s.pending)
	s.pending = s.pending[n:]
	if len(s.pending) == 0 {
		return n, s.err
	}
	return n, nil
}

func (s *deviceDisplayStream) Close() error { return s.source.Close() }

func displayDeviceResponse(response *http.Response) error {
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		response.Body = &deviceDisplayStream{source: response.Body, reader: bufio.NewReader(response.Body)}
		response.ContentLength = -1
		response.Header.Del("Content-Length")
	} else {
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return err
		}
		body := displayDeviceJSON(raw)
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	response.Header.Del("ETag")
	response.Header.Set("Cache-Control", "no-store")
	return nil
}
