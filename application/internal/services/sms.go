package services

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SMS sends text messages via Twilio's REST API — plain net/http and
// Basic Auth rather than pulling in Twilio's SDK, since a single POST is
// all this integration needs.
type SMS struct {
	AccountSID string
	AuthToken  string
	FromNumber string
}

// toE164 formats a US phone number (any of the dashed/plain forms
// normalizePhoneNumber in players.go already produces) as E.164 for
// Twilio — this app's phone numbers are US-only today, the same
// assumption normalizeZip/state-code validation already make.
func toE164(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if len(d) == 11 && strings.HasPrefix(d, "1") {
		return "+" + d
	}
	return "+1" + d
}

// Send texts body to a US phone number.
func (s *SMS) Send(to, body string) error {
	form := url.Values{
		"To":   {toE164(to)},
		"From": {toE164(s.FromNumber)},
		"Body": {body},
	}

	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", s.AccountSID)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.AccountSID, s.AuthToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio send error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio send error: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
