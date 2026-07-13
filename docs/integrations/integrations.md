# Integrations

## Mailjet

**Purpose:** All transactional email.
**API:** Mailjet REST API v3
**Service:** `internal/services/email.go` — `Email` struct

### Configuration

| Variable | Purpose |
|---|---|
| `EMAIL_USER` | Mailjet API key |
| `EMAIL_PASSWORD` | Mailjet secret key |
| `EMAIL_SENDER` | From address (must be validated in Mailjet) |

If `EMAIL_USER` is unset, `main.go` never constructs an `Email` service — `UserService.Email` and `app.emailService` stay `nil`, and every call site checks for that before sending, so dev/CI environments without Mailjet credentials degrade gracefully (email sending is skipped, not fatal). This is a fix carried forward from the reference project, where the `Email` service was always constructed even with blank credentials, so those nil-checks never actually protected anything.

### Sending: `SendEmailV2`

```go
func (email *Email) SendEmailV2(subject, mime, body, recipient string) error {
    mj := mailjet.NewMailjetClient(email.Username, email.Password)
    // builds MessagesV31, calls mj.SendMailV31(&messages)
}
```

The [Mailjet Go SDK](https://github.com/mailjet/mailjet-apiv3-go) wraps the Mailjet v3.1 messages API. The `From` address must be a sender validated in the Mailjet account. This is the only send path — the reference project's legacy `net/smtp`-based `SendEmail`/`SMTPClient` code (dead code there) was not ported.

**Transactional emails sent by the application:**

| Trigger | Recipient | Subject |
|---|---|---|
| User signup | New user | Activate your League Buddy account (activation link) |
| Forgot password | User | League Buddy Password Reset (reset link) |
| Admin adds a player with an email | New player | You've been added to the League Buddy roster (signup link) |

No bulk/batch email system is ported — the reference project's `EmailTemplate`/`EmailBatch` admin-authored bulk email feature isn't needed for a single team roster.

---

## PayPal — not yet integrated

Deliberately out of scope for this scaffold. When team payments (dues, fees, tournament costs) are needed, `toller-club-docker`'s `internal/services/paypal.go` (OAuth2 client-credentials REST API v2, order-create + capture flow) and its two `/api/orders` handlers in `handlers_site.go` are a working reference implementation to port over. See that project's `docs/integrations/integrations.md` for the full flow documentation.
