package extcal

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tschuyebuhl/ews"
	"github.com/tschuyebuhl/ews/ewsutil"
)

func guessEWSEndpoints(email string) []string {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.LastIndex(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return []string{"https://outlook.office365.com/EWS/Exchange.asmx"}
	}
	domain := email[at+1:]
	var out []string
	// O365 first
	out = append(out, "https://outlook.office365.com/EWS/Exchange.asmx")
	out = append(out,
		"https://mail."+domain+"/EWS/Exchange.asmx",
		"https://exchange."+domain+"/EWS/Exchange.asmx",
		"https://"+domain+"/EWS/Exchange.asmx",
	)
	return out
}

func normalizeEWSURL(raw string) (string, bool) {
	raw = ensureHTTPSScheme(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if u.Host == "" {
		return "", false
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/EWS/Exchange.asmx"
	}
	return u.String(), true
}

func listExchangeEventsWithContext(ctx context.Context, ep, email, password string, ntlm bool, from time.Time, duration time.Duration) ([]ParsedEvent, error) {
	type result struct {
		events []ParsedEvent
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		c := ews.NewClient(ep, email, password, &ews.Config{Dump: false, NTLM: ntlm, SkipTLS: false})
		users := []ewsutil.EventUser{{Email: email, AttendeeType: ews.AttendeeTypeRequired}}
		m, err := ewsutil.ListUsersEvents(c, users, from, duration)
		if err != nil {
			ch <- result{err: err}
			return
		}
		evs := m[users[0]]
		out := make([]ParsedEvent, 0, len(evs))
		for _, e := range evs {
			title := "(忙碌)"
			if string(e.BusyType) != "" {
				title = fmt.Sprintf("(忙碌：%s)", e.BusyType)
			}
			out = append(out, ParsedEvent{
				Title:  title,
				Start:  e.Start,
				End:    e.End,
				AllDay: false,
			})
		}
		ch <- result{events: out}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.events, r.err
	}
}

// FetchExchangeBusyEvents pulls free/busy blocks for the given email.
// Note: Exchange EWS GetUserAvailability returns busy blocks without subjects.
func FetchExchangeBusyEvents(ctx context.Context, ewsURL, email, password string, from, toInclusive time.Time) ([]ParsedEvent, error) {
	email = strings.TrimSpace(email)
	if email == "" || strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("missing credentials")
	}
	var endpoints []string
	if s, ok := normalizeEWSURL(ewsURL); ok {
		endpoints = []string{s}
	} else {
		endpoints = guessEWSEndpoints(email)
	}

	// 120 days max already enforced by handler; map to duration.
	duration := toInclusive.Add(24 * time.Hour).Sub(from)
	if duration <= 0 {
		return nil, fmt.Errorf("invalid range")
	}

	var lastErr error
	for _, ep := range endpoints {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Try NTLM first (common on-prem), then basic.
		for _, ntlm := range []bool{true, false} {
			out, err := listExchangeEventsWithContext(ctx, ep, email, password, ntlm, from, duration)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				lastErr = err
				continue
			}
			return out, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unable to connect to exchange")
	}
	return nil, lastErr
}
