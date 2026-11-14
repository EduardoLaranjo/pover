package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const (
	testUser  = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	testToken = "azGDORePK8gMaC0QOYAMyEEuzJnyUi"
)

var testURL = "pover://" + testUser + "@" + testToken

// fakePushover records the forms it receives and answers with status.
func fakePushover(t *testing.T, status int, reply string) (*httptest.Server, *[]url.Values) {
	t.Helper()

	var got []url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}

		got = append(got, r.PostForm)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)

	return srv, &got
}

func post(t *testing.T, mux http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	return rec
}

func notifyBody(t *testing.T, urls, title, body, typ string) string {
	t.Helper()

	b, err := json.Marshal(notifyRequest{URLs: urls, Title: title, Body: body, Type: typ})
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

// TestDocoCDRequest sends the body Doco-CD v0.134.0 sends.
func TestDocoCDRequest(t *testing.T) {
	srv, got := fakePushover(t, http.StatusOK, `{"status":1}`)
	mux := newMux(newPushover(srv.URL))

	rec := post(t, mux, notifyBody(t, testURL, "✅ Deployment completed", "stack: grafana", "success"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	if len(*got) != 1 {
		t.Fatalf("pushover calls = %d, want 1", len(*got))
	}

	form := (*got)[0]
	for key, want := range map[string]string{
		"token":   testToken,
		"user":    testUser,
		"title":   "✅ Deployment completed",
		"message": "stack: grafana",
	} {
		if form.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, form.Get(key), want)
		}
	}

	for _, key := range []string{"device", "priority", "sound", "url", "url_title"} {
		if form.Has(key) {
			t.Errorf("unexpected %s = %q", key, form.Get(key))
		}
	}
}

func TestPushoverFailureIs424WithoutCredentials(t *testing.T) {
	srv, _ := fakePushover(t, http.StatusBadRequest, `{"status":0,"errors":["application token is invalid"]}`)
	mux := newMux(newPushover(srv.URL))

	rec := post(t, mux, notifyBody(t, testURL, "t", "b", "failure"))

	if rec.Code != http.StatusFailedDependency {
		t.Fatalf("status = %d, want 424", rec.Code)
	}

	if strings.Contains(rec.Body.String(), testToken) || strings.Contains(rec.Body.String(), testUser) {
		t.Errorf("response leaks credentials: %s", rec.Body)
	}
}

func TestBadRequests(t *testing.T) {
	srv, got := fakePushover(t, http.StatusOK, `{"status":1}`)
	mux := newMux(newPushover(srv.URL))

	cases := map[string]string{
		"not JSON":           "urls=x",
		"no body":            notifyBody(t, testURL, "t", " ", ""),
		"no urls":            notifyBody(t, "", "t", "b", ""),
		"bad type":           notifyBody(t, testURL, "t", "b", "error"),
		"unsupported scheme": notifyBody(t, "tgram://bot/chat", "t", "b", ""),
		"short user key":     notifyBody(t, "pover://short@"+testToken, "t", "b", ""),
		"unknown parameter":  notifyBody(t, testURL+"?retry=60", "t", "b", ""),
		"emergency priority": notifyBody(t, testURL+"?priority=emergency", "t", "b", ""),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := post(t, mux, body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}

			if strings.Contains(rec.Body.String(), testToken) {
				t.Errorf("response leaks the token: %s", rec.Body)
			}
		})
	}

	if len(*got) != 0 {
		t.Errorf("pushover called %d times for bad requests", len(*got))
	}
}

func TestOptionsAndSeveralURLs(t *testing.T) {
	srv, got := fakePushover(t, http.StatusOK, `{"status":1}`)
	mux := newMux(newPushover(srv.URL))

	urls := testURL + "/phone/laptop?priority=high&sound=siren&url=https://example.com&url_title=Open, " + testURL
	rec := post(t, mux, notifyBody(t, urls, "", strings.Repeat("é", 2000), ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	if len(*got) != 2 {
		t.Fatalf("pushover calls = %d, want 2", len(*got))
	}

	form := (*got)[0]
	for key, want := range map[string]string{
		"device":    "phone,laptop",
		"priority":  "1",
		"sound":     "siren",
		"url":       "https://example.com",
		"url_title": "Open",
	} {
		if form.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, form.Get(key), want)
		}
	}

	if n := len([]rune(form.Get("message"))); n != pushoverMessageMax {
		t.Errorf("message length = %d runes, want %d", n, pushoverMessageMax)
	}

	if form.Has("title") {
		t.Errorf("empty title was sent")
	}
}

func TestStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
