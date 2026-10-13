// SPDX-FileCopyrightText: (c) Mauve Mailorder Software GmbH & Co. KG, 2022. Licensed under [MIT](LICENSE) license.
//
// SPDX-License-Identifier: MIT

// Package fakeilo provides a fake iLO Redfish service for tests. It serves
// static mockups in the DMTF/HPE mockup layout (<dir>/<path>/index.json, with
// <dir>/index.json being /redfish/v1/) and emulates the parts of iLO behavior
// the exporter depends on: Basic and session auth, $expand=. and error codes.
package fakeilo

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const prefix = "/redfish/v1"

// Server is a fake iLO Redfish service
type Server struct {
	srv  *httptest.Server
	root *os.Root

	username string
	password string
	expand   bool
	latency  time.Duration
	status   map[string]int
	block    map[string]bool
	hidden   map[string]map[string]bool

	mu          sync.Mutex
	conns       int
	requests    []string
	inFlight    int
	maxInFlight int
	sessions    map[string]string
	sessionSeq  int
	logins      int
}

// Option configures a Server
type Option func(*Server)

// WithCredentials requires Basic or session auth with the given credentials
func WithCredentials(username, password string) Option {
	return func(s *Server) {
		s.username = username
		s.password = password
	}
}

// WithoutExpand makes the server ignore $expand, like services that do not implement it
func WithoutExpand() Option {
	return func(s *Server) {
		s.expand = false
	}
}

// WithLatency delays every response
func WithLatency(d time.Duration) Option {
	return func(s *Server) {
		s.latency = d
	}
}

// WithStatus makes requests for path (e.g. /redfish/v1/Chassis/1/Power) fail with code
func WithStatus(path string, code int) Option {
	return func(s *Server) {
		s.status[normalize(path)] = code
	}
}

// WithBlock makes requests for path hang until the client gives up
func WithBlock(path string) Option {
	return func(s *Server) {
		s.block[normalize(path)] = true
	}
}

// WithoutProperty removes a top-level property from the resource at path,
// e.g. to simulate a Chassis without legacy Power/Thermal links
func WithoutProperty(path, property string) Option {
	return func(s *Server) {
		p := normalize(path)
		if s.hidden[p] == nil {
			s.hidden[p] = make(map[string]bool)
		}
		s.hidden[p][property] = true
	}
}

// New starts a TLS server serving the mockup in dir
func New(t testing.TB, dir string, opts ...Option) *Server {
	t.Helper()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("could not open mockup: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	s := &Server{
		root:     root,
		expand:   true,
		status:   make(map[string]int),
		block:    make(map[string]bool),
		hidden:   make(map[string]map[string]bool),
		sessions: make(map[string]string),
	}
	for _, o := range opts {
		o(s)
	}

	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	s.srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			s.mu.Lock()
			s.conns++
			s.mu.Unlock()
		}
	}
	s.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.srv.StartTLS()
	t.Cleanup(s.srv.Close)

	return s
}

// Host returns host:port of the server
func (s *Server) Host() string {
	return strings.TrimPrefix(s.srv.URL, "https://")
}

// Certificate returns the certificate of the server
func (s *Server) Certificate() *x509.Certificate {
	return s.srv.Certificate()
}

// Connections returns the number of connections accepted so far
func (s *Server) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.conns
}

// Requests returns all requests received so far as "METHOD path[?query]", sorted
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := append([]string(nil), s.requests...)
	sort.Strings(r)
	return r
}

// ResetRequests clears the recorded requests
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = nil
}

// MaxInFlight returns the highest number of concurrently handled requests
func (s *Server) MaxInFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.maxInFlight
}

// InFlight returns the number of requests currently being handled
func (s *Server) InFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.inFlight
}

// Sessions returns the number of open sessions
func (s *Server) Sessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.sessions)
}

// Logins returns the number of successful session logins
func (s *Server) Logins() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.logins
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	req := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		q, _ := url.QueryUnescape(r.URL.RawQuery)
		req += "?" + q
	}
	s.requests = append(s.requests, req)
	s.inFlight++
	if s.inFlight > s.maxInFlight {
		s.maxInFlight = s.inFlight
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()

	if s.latency > 0 {
		select {
		case <-time.After(s.latency):
		case <-r.Context().Done():
			return
		}
	}

	p := normalize(r.URL.Path)
	if s.block[p] {
		<-r.Context().Done()
		return
	}

	if p == prefix+"/SessionService/Sessions" && r.Method == http.MethodPost {
		s.login(w, r)
		return
	}

	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "Base.1.0.NoValidSession")
		return
	}

	if r.Method == http.MethodDelete && strings.HasPrefix(p, prefix+"/SessionService/Sessions/") {
		s.logout(w, p)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Base.1.0.OperationNotAllowed")
		return
	}

	if code, found := s.status[p]; found {
		writeError(w, code, "Base.1.0.InternalError")
		return
	}

	res, err := s.load(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI")
		return
	}

	if e := r.URL.Query().Get("$expand"); e != "" {
		switch e {
		case ".", ".($levels=1)":
			if s.expand {
				s.expandRefs(res)
			}
		default:
			// iLO < 7 1.22 rejects every other $expand value
			writeError(w, http.StatusBadRequest, "Base.1.19.QueryParameterValueFormatError")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) authorized(r *http.Request) bool {
	if s.username == "" {
		return true
	}

	if token := r.Header.Get("X-Auth-Token"); token != "" {
		s.mu.Lock()
		defer s.mu.Unlock()

		for _, t := range s.sessions {
			if t == token {
				return true
			}
		}
		return false
	}

	u, p, ok := r.BasicAuth()
	return ok && u == s.username && p == s.password
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		UserName string
		Password string
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "Base.1.0.MalformedJSON")
		return
	}

	if s.username != "" && (creds.UserName != s.username || creds.Password != s.password) {
		writeError(w, http.StatusUnauthorized, "Base.1.0.NoValidSession")
		return
	}

	s.mu.Lock()
	s.sessionSeq++
	s.logins++
	id := fmt.Sprintf("%s/SessionService/Sessions/session%d", prefix, s.sessionSeq)
	token := fmt.Sprintf("token%d", s.sessionSeq)
	s.sessions[id] = token
	s.mu.Unlock()

	w.Header().Set("X-Auth-Token", token)
	w.Header().Set("Location", "https://"+r.Host+id+"/")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"@odata.id": id + "/"})
}

func (s *Server) logout(w http.ResponseWriter, p string) {
	s.mu.Lock()
	_, found := s.sessions[p]
	delete(s.sessions, p)
	s.mu.Unlock()

	if !found {
		writeError(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ExpireSessions invalidates all sessions, like an iLO idle timeout would
func (s *Server) ExpireSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions = make(map[string]string)
}

func (s *Server) load(p string) (map[string]any, error) {
	rel := strings.TrimPrefix(strings.TrimPrefix(p, prefix), "/")
	b, err := s.root.ReadFile(path.Join(rel, "index.json"))
	if err != nil {
		return nil, err
	}

	res := make(map[string]any)
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}

	for prop := range s.hidden[p] {
		delete(res, prop)
	}

	return res, nil
}

// expandRefs replaces top-level references (except in Links) with the referenced resources
func (s *Server) expandRefs(res map[string]any) {
	for k, v := range res {
		if k == "Links" || strings.HasPrefix(k, "@") {
			continue
		}

		switch val := v.(type) {
		case map[string]any:
			if r := s.resolve(val); r != nil {
				res[k] = r
			}
		case []any:
			for i, item := range val {
				if m, ok := item.(map[string]any); ok {
					if r := s.resolve(m); r != nil {
						val[i] = r
					}
				}
			}
		}
	}
}

func (s *Server) resolve(ref map[string]any) map[string]any {
	if len(ref) != 1 {
		return nil
	}

	id, ok := ref["@odata.id"].(string)
	if !ok {
		return nil
	}

	r, err := s.load(normalize(id))
	if err != nil {
		return nil
	}

	return r
}

func normalize(p string) string {
	return strings.TrimSuffix(p, "/")
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "iLO.0.10.ExtendedInfo",
			"message": "See @Message.ExtendedInfo for more information.",
			"@Message.ExtendedInfo": []map[string]string{
				{"MessageId": msg},
			},
		},
	})
}
