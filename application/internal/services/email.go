package services

import (
	"fmt"

	mailjet "github.com/mailjet/mailjet-apiv3-go/v4"
)

type Email struct {
	Username string
	Password string
	Sender   string
}

func (email *Email) SendEmailV2(subject, mime, body, recipient string) error {

	mj := mailjet.NewMailjetClient(email.Username, email.Password)

	messagesInfo := []mailjet.InfoMessagesV31{
		{
			From: &mailjet.RecipientV31{
				Email: email.Sender, // Must be validated with Mailjet
				Name:  "League Buddy",
			},
			To: &mailjet.RecipientsV31{
				mailjet.RecipientV31{
					Email: recipient,
					Name:  "",
				},
			},
			Subject:  subject,
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
