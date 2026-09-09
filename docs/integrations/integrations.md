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
| Admin/captain adds a player with an email | New player | You've been added to the League Buddy roster (signup link) |
| Captain/admin sends a team invite | Each invited address | You're invited to join `<team>` (signup link carrying the invite token) |
| Player requests to join a team | That team's captain, if assigned and has an email on file | New join request for `<team>` (review link) |

No bulk/batch email system is ported — the reference project's `EmailTemplate`/`EmailBatch` admin-authored bulk email feature isn't needed here.

---

## Twilio (SMS)

**Purpose:** Phone-number verification codes and text-message match/RSVP reminders.
**API:** Twilio Messages API (`https://api.twilio.com/2010-04-01/Accounts/{SID}/Messages.json`) via plain `net/http` + Basic Auth — no Twilio SDK dependency.
**Service:** `internal/services/sms.go` — `SMS` struct (`AccountSID`, `AuthToken`, `FromNumber`), one method `Send(to, body string) error`. Phone numbers are normalized to E.164 before sending.

### Configuration

| Variable | Purpose |
|---|---|
| `SMS_FEATURE_ENABLED` | Site-wide flag gating whether the phone-verification/notification-preferences UI is shown **at all** — deliberately separate from the credentials below, so real Twilio creds can sit configured (e.g. for backend testing while a toll-free number awaits carrier approval) without exposing a "verify your phone" flow to real users it can't yet deliver to |
| `SMS_ACCOUNT_SID`, `SMS_AUTH_TOKEN` | Twilio account credentials |
| `SMS_FROM_NUMBER` | An SMS-capable Twilio number, in E.164 format |

If `SMS_ACCOUNT_SID` is unset, `main.go` never constructs an `SMS` service — verification codes and reminders are logged instead of sent, not fatal — the same degrade-gracefully pattern `EMAIL_USER` uses above.

### Consent model

Texting a player requires all three of the following, each independently checked wherever a message is about to be sent (`MatchReminderService.notify`, `SendTestReminderSMS`, and its own recipient picker) — not just at the form that sets them:

1. **Phone verified** (`Player.PhoneVerifiedAt`) — proves the player controls the number, via a 6-digit code sent to it and confirmed back.
2. **SMS program opt-in** (`Player.SMSOptInAt`) — a separate, explicit, persisted consent event distinct from verification, recorded when the player checks the opt-in box while requesting a code. Revocable at any time via a self-service "Opt Out of SMS Program" action, independent of removing the phone number itself.
3. **Per-category delivery preference** (`playerNotificationPreferences` table, `models.ChannelSMS`/`ChannelBoth`) — a player can enable text for RSVP reminders and/or captain's messages independently; neither is implied by the other, and both default to email-only until changed.

A preference save that requests SMS/Both without (1) and (2) both being true is rejected server-side (`NotificationPreferenceService.SetPreference`) regardless of what the form itself renders. STOP/HELP keyword replies are handled entirely by Twilio's own Advanced Opt-Out feature at the messaging-service level — the app has no inbound-SMS webhook and never itself sees or records a STOP reply.

**Transactional/automated texts sent by the application:**

| Trigger | Recipient | Content |
|---|---|---|
| Phone verification requested | The player, at the number just entered | 6-digit one-time code |
| RSVP reminder due (per team's configured schedule) | Roster players who haven't responded and prefer text | Match date/opponent, RSVP link |
| Captain's-message reminder due | Roster players who prefer text | The captain's message for that match |
| Captain/admin "Send Test Reminder" (SMS) | Selected verified, opted-in teammates | A `[TEST]`-prefixed preview of the real reminder copy |

---

## PayPal — not yet integrated

Deliberately out of scope for this scaffold. When team payments (dues, fees, tournament costs) are needed, `toller-club-docker`'s `internal/services/paypal.go` (OAuth2 client-credentials REST API v2, order-create + capture flow) and its two `/api/orders` handlers in `handlers_site.go` are a working reference implementation to port over. See that project's `docs/integrations/integrations.md` for the full flow documentation.
