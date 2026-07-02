package email

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v3"
)

type ResendSender struct {
	client     *resend.Client
	from       string
	templateID string
}

func NewResendSender(apiKey, from, templateID string) *ResendSender {
	return &ResendSender{client: resend.NewClient(apiKey), from: from, templateID: templateID}
}

func (s *ResendSender) SendVerificationEmail(ctx context.Context, to, username, verifyURL string) error {
	_, err := s.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From: s.from,
		To:   []string{to},
		Template: &resend.EmailTemplate{
			Id: s.templateID,
			Variables: map[string]any{
				"username":   username,
				"verify_url": verifyURL,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("resend send: %w", err)
	}
	return nil
}
