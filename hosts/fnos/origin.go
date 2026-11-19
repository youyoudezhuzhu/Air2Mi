package main

import (
	"net/http"
	"net/url"
	"strings"
)

// fnOS may replace Host when proxying to the Unix socket. On that transport
// admin authentication and a secret session nonce are the security boundary;
// comparing the public browser Origin with the internal Host rejects valid UI.
// Forwarded headers are never accepted as proof of authority.
func (s *Server) validOrigin(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // native clients still require authentication + CSRF
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	trusted, _ := r.Context().Value(peerKey{}).(bool)
	if s.devToken == "" && trusted && s.authorized(r) {
		return true
	}
	return strings.EqualFold(u.Host, r.Host)
}
