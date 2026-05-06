// Phase 0 webhook placeholders.
//
// Public, unauthenticated endpoints whose only job is to satisfy the Phase 0
// exit criterion: prove that a Stripe or WorkOS test event sent at our
// Cloudflare Tunnel hostname actually reaches the API. They read the body,
// log size + a few headers, and return 200.
//
// They do NOT verify signatures yet. Phase 1 wires real verification on
// /webhooks/workos; Phase 5 wires it on /webhooks/stripe.

package httpapi

import (
	"io"
	"net/http"
)

const maxWebhookBytes = 1 << 20 // 1 MiB; both providers stay well under

func (d *Deps) stripeWebhookPlaceholder(w http.ResponseWriter, r *http.Request) {
	d.logWebhook(r, "stripe")
	w.WriteHeader(http.StatusOK)
}

func (d *Deps) workosWebhookPlaceholder(w http.ResponseWriter, r *http.Request) {
	d.logWebhook(r, "workos")
	w.WriteHeader(http.StatusOK)
}

func (d *Deps) logWebhook(r *http.Request, provider string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes))
	_ = r.Body.Close()

	fields := map[string]any{
		"provider":     provider,
		"bytes":        len(body),
		"content_type": r.Header.Get("Content-Type"),
		"user_agent":   r.Header.Get("User-Agent"),
	}
	if id := r.Header.Get("Stripe-Signature"); id != "" {
		fields["stripe_signature_present"] = true
	}
	if id := r.Header.Get("WorkOS-Signature"); id != "" {
		fields["workos_signature_present"] = true
	}
	if err != nil {
		fields["read_err"] = err.Error()
		d.Log.Error(r.Context(), "webhook read error", fields)
		return
	}
	d.Log.Info(r.Context(), "webhook received", fields)
}
