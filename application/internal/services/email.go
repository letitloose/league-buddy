package services

import (
	"fmt"
	"regexp"
	"strings"

	mailjet "github.com/mailjet/mailjet-apiv3-go/v4"
)

type Email struct {
	Username string
	Password string
	Sender   string
}

var (
	anchorTagRegexp = regexp.MustCompile(`<a\s+href="([^"]*)"[^>]*>([^<]*)</a>`)
	htmlTagRegexp   = regexp.MustCompile(`<[^>]*>`)
)

// textFromHTML derives a plain-text fallback from a simple HTML email body —
// links become "text (url)", every other tag is stripped, and whitespace is
// collapsed. Sending HTML with no plain-text alternative is itself a spam
// signal (especially to Microsoft's filters), so every send gets one
// alongside HTMLPart. Good enough for our own hand-authored, single-link
// bodies — not a general-purpose HTML sanitizer.
func textFromHTML(html string) string {
	text := anchorTagRegexp.ReplaceAllString(html, "$2 ($1)")
	text = htmlTagRegexp.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(text), " ")
}

func (email *Email) SendEmailV2(subject, mime, body, recipient string) error {

	mj := mailjet.NewMailjetClient(email.Username, email.Password)

	messagesInfo := []mailjet.InfoMessagesV31{
		{
			From: &mailjet.RecipientV31{
				Email: email.Sender, // Must be validated with Mailjet
				Name:  "Blame the Ball",
			},
			To: &mailjet.RecipientsV31{
				mailjet.RecipientV31{
					Email: recipient,
					Name:  "",
				},
			},
			Subject:  subject,
			TextPart: textFromHTML(body),
			HTMLPart: body,
		},
	}

	messages := mailjet.MessagesV31{Info: messagesInfo}

	_, err := mj.SendMailV31(&messages)
	if err != nil {
		return fmt.Errorf("mailjet send error: %w", err)
	}

	return nil
}
