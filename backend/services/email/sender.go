package email

import "context"

type Sender interface {
	SendVerificationEmail(ctx context.Context, to, username, verifyURL string) error
}
