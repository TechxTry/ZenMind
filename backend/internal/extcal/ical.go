package extcal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"
)

const maxICSBytes = 5 << 20

// ParsedEvent is one VEVENT instance (recurrence not expanded).
type ParsedEvent struct {
	Title  string
	Start  time.Time
	End    time.Time
	AllDay bool
}

func ValidateCalendarURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https are allowed")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host")
	}
	return u, nil
}

// FeedHost returns a short display host for list UI.
func FeedHost(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(u.Host))
}

// FetchICS downloads calendar data with size and timeout limits.
func FetchICS(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := ValidateCalendarURL(rawURL)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZenMind/1.0 (+https://github.com/TechxTry/ZenMind) calendar-fetch")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxICSBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxICSBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxICSBytes)
	}
	return body, nil
}

func eventTitle(ev *ical.VEvent) string {
	p := ev.GetProperty(ical.ComponentPropertySummary)
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Value)
}

func parseOneEvent(ev *ical.VEvent) (ParsedEvent, bool) {
	var out ParsedEvent
	out.Title = eventTitle(ev)
	if out.Title == "" {
		out.Title = "(无标题)"
	}

	// All-day: prefer explicit all-day parsing
	if s, err := ev.GetAllDayStartAt(); err == nil {
		out.AllDay = true
		out.Start = s
		if e, err2 := ev.GetAllDayEndAt(); err2 == nil && !e.Before(s) {
			out.End = e
		} else {
			out.End = s.Add(24 * time.Hour)
		}
		return out, true
	}

	start, err := ev.GetStartAt()
	if err != nil {
		return ParsedEvent{}, false
	}
	out.Start = start
	out.AllDay = false
	if end, err2 := ev.GetEndAt(); err2 == nil && !end.Before(start) {
		out.End = end
	} else {
		out.End = start.Add(time.Hour)
	}
	return out, true
}

func intersectsWindow(evStart, evEnd, winStart, winEndExclusive time.Time) bool {
	if evEnd.Before(evStart) {
		evEnd = evStart
	}
	return evStart.Before(winEndExclusive) && !evEnd.Before(winStart)
}

// EventsInWindow parses ICS bytes and returns events overlapping [winStart, winEndInclusive] by calendar date.
func EventsInWindow(body []byte, winStart, winEndInclusive time.Time) ([]ParsedEvent, error) {
	cal, err := ical.ParseCalendar(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	winEndExclusive := winEndInclusive.Add(24 * time.Hour)
	var out []ParsedEvent
	for _, ev := range cal.Events() {
		parsed, ok := parseOneEvent(ev)
		if !ok {
			continue
		}
		if !intersectsWindow(parsed.Start, parsed.End, winStart, winEndExclusive) {
			continue
		}
		out = append(out, parsed)
	}
	return out, nil
}
