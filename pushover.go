package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	pushoverAPI = "https://api.pushover.net/1/messages.json"

	// Pushover's limits; longer text is cut rather than rejected.
	pushoverTitleMax   = 250
	pushoverMessageMax = 1024
)

var pushoverKey = regexp.MustCompile(`^[A-Za-z0-9]{30}$`)

// Apprise's priority names. Emergency (2) needs retry and expire, which pover
// does not support yet.
var pushoverPriorities = map[string]int{
	"low":      -2,
	"moderate": -1,
	"normal":   0,
	"high":     1,
}

type pushover struct {
	endpoint string
	client   *http.Client
}

func newPushover(endpoint string) *pushover {
	return &pushover{endpoint: endpoint, client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *pushover) scheme() string { return "pover" }

type pushoverTarget struct {
	p        *pushover
	user     string
	token    string
	devices  []string
	priority int
	sound    string
	link     string
	linkText string
}

// parse reads pover://USER_KEY@APP_TOKEN[/DEVICE...][?priority=&sound=&url=&url_title=].
func (p *pushover) parse(u *url.URL) (target, error) {
	if u.User == nil {
		return nil, errors.New("pover URL needs USER_KEY@APP_TOKEN")
	}

	t := &pushoverTarget{p: p, user: u.User.Username(), token: u.Host}

	if _, hasPassword := u.User.Password(); hasPassword {
		return nil, errors.New("pover URL takes USER_KEY@APP_TOKEN, not a password")
	}

	if !pushoverKey.MatchString(t.user) {
		return nil, errors.New("pover user key must be 30 letters or digits")
	}

	if !pushoverKey.MatchString(t.token) {
		return nil, errors.New("pover app token must be 30 letters or digits")
	}

	for _, d := range strings.Split(u.Path, "/") {
		if d != "" {
			t.devices = append(t.devices, d)
		}
	}

	for key, values := range u.Query() {
		v := values[len(values)-1]

		switch key {
		case "priority":
			prio, err := parsePushoverPriority(v)
			if err != nil {
				return nil, err
			}

			t.priority = prio
		case "sound":
			t.sound = v
		case "url":
			t.link = v
		case "url_title":
			t.linkText = v
		default:
			return nil, fmt.Errorf("pover URL has unsupported parameter %q", key)
		}
	}

	return t, nil
}

func parsePushoverPriority(v string) (int, error) {
	if prio, ok := pushoverPriorities[strings.ToLower(v)]; ok {
		return prio, nil
	}

	prio, err := strconv.Atoi(v)
	if err != nil || prio < -2 || prio > 1 {
		return 0, fmt.Errorf("pover priority %q must be low, moderate, normal, high or -2 to 1", v)
	}

	return prio, nil
}

func (t *pushoverTarget) send(r *http.Request, m message) error {
	form := url.Values{
		"token":   {t.token},
		"user":    {t.user},
		"message": {truncate(m.Body, pushoverMessageMax)},
	}

	if m.Title != "" {
		form.Set("title", truncate(m.Title, pushoverTitleMax))
	}

	if len(t.devices) > 0 {
		form.Set("device", strings.Join(t.devices, ","))
	}

	if t.priority != 0 {
		form.Set("priority", strconv.Itoa(t.priority))
	}

	if t.sound != "" {
		form.Set("sound", t.sound)
	}

	if t.link != "" {
		form.Set("url", t.link)
	}

	if t.linkText != "" {
		form.Set("url_title", t.linkText)
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, t.p.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.p.client.Do(req)
	if err != nil {
		// The error text includes the endpoint, never the form with the credentials.
		return fmt.Errorf("pushover request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	var body struct {
		Errors []string `json:"errors"`
	}

	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)

	if len(body.Errors) > 0 {
		return fmt.Errorf("pushover: %s: %s", resp.Status, strings.Join(body.Errors, "; "))
	}

	return fmt.Errorf("pushover: %s", resp.Status)
}

// truncate cuts s to at most max characters, ending with an ellipsis when cut.
func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}

	runes := []rune(s)

	return string(runes[:max-1]) + "…"
}
