package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxRequestBytes = 1 << 20

// notifyRequest is the Apprise API's stateless /notify body.
type notifyRequest struct {
	URLs  string `json:"urls"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Type  string `json:"type"`
}

// message is what a service sends; Type is info, success, warning or failure.
type message struct {
	Title string
	Body  string
	Type  string
}

// target is one parsed notify URL, ready to send.
type target interface {
	send(r *http.Request, m message) error
}

// service parses the notify URLs of one scheme.
type service interface {
	scheme() string
	parse(u *url.URL) (target, error)
}

func newMux(services ...service) *http.ServeMux {
	byScheme := make(map[string]service, len(services))
	for _, s := range services {
		byScheme[s.scheme()] = s
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /notify", func(w http.ResponseWriter, r *http.Request) {
		handleNotify(w, r, byScheme)
	})

	return mux
}

func handleNotify(w http.ResponseWriter, r *http.Request, byScheme map[string]service) {
	var req notifyRequest

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	switch req.Type {
	case "":
		req.Type = "info"
	case "info", "success", "warning", "failure":
	default:
		writeError(w, http.StatusBadRequest, "type must be info, success, warning or failure")
		return
	}

	targets, err := parseTargets(req.URLs, byScheme)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	m := message{Title: req.Title, Body: req.Body, Type: req.Type}

	var failed int
	for i, t := range targets {
		if err := t.send(r, m); err != nil {
			failed++
			// Errors never carry the URL, which holds credentials.
			slog.Error("delivery failed", "target", i, "error", err)
		}
	}

	if failed > 0 {
		// Apprise answers 424 when any delivery fails.
		writeError(w, http.StatusFailedDependency, "one or more notifications could not be sent")
		return
	}

	slog.Info("delivered", "targets", len(targets), "type", m.Type)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// parseTargets splits Apprise's URL list on commas and whitespace.
func parseTargets(raw string, byScheme map[string]service) ([]target, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) == 0 {
		return nil, errors.New("urls is required")
	}

	targets := make([]target, 0, len(fields))
	for i, f := range fields {
		// Report URLs by position only: they hold credentials.
		pos := "url " + strconv.Itoa(i)

		u, err := url.Parse(f)
		if err != nil {
			return nil, errors.New(pos + " is not a valid URL")
		}

		s, ok := byScheme[strings.ToLower(u.Scheme)]
		if !ok {
			return nil, errors.New(pos + " has an unsupported scheme " + u.Scheme)
		}

		t, err := s.parse(u)
		if err != nil {
			return nil, errors.New(pos + ": " + err.Error())
		}

		targets = append(targets, t)
	}

	return targets, nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
