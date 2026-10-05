package notify

import (
	"context"
	"fmt"
	"strings"
)

// Sender sends one SMS and returns the provider's message id.
type Sender interface {
	Send(context.Context, SMS) (string, error)
}

// ProviderFromEnv picks the SMS provider from the environment and returns it with its name.
//
//   - SMS_PROVIDER=gateway or twilio forces that provider. If it is misconfigured the result is an error, never a
//     quiet switch to the other one, because a text sent from an unexpected number is worse than no text.
//   - With SMS_PROVIDER empty, our own cellular gateway is used when GATEWAY_URL or GATEWAY_API_KEY is set,
//     otherwise Twilio.
func ProviderFromEnv(env func(string) string) (Sender, string, error) {
	provider := strings.ToLower(strings.TrimSpace(env("SMS_PROVIDER")))
	gatewaySet := strings.TrimSpace(env("GATEWAY_URL")) != "" || strings.TrimSpace(env("GATEWAY_API_KEY")) != ""
	switch {
	case provider == "gateway" || (provider == "" && gatewaySet):
		g, err := NewGateway(GatewayConfig{
			BaseURL: env("GATEWAY_URL"),
			APIKey:  env("GATEWAY_API_KEY"),
			Group:   env("GATEWAY_GROUP"),
			Device:  env("GATEWAY_DEVICE"),
			Queue:   strings.ToLower(strings.TrimSpace(env("GATEWAY_QUEUE"))) != "false",
		}, nil)
		if err != nil {
			return nil, "gateway", err
		}
		return g, "gateway", nil
	case provider == "" || provider == "twilio":
		t, err := NewTwilio(TwilioConfig{
			AccountSID:          env("TWILIO_ACCOUNT_SID"),
			AuthToken:           env("TWILIO_AUTH_TOKEN"),
			From:                env("TWILIO_FROM"),
			MessagingServiceSID: env("TWILIO_MESSAGING_SERVICE_SID"),
			StatusCallbackURL:   env("TWILIO_STATUS_CALLBACK_URL"),
		}, nil)
		if err != nil {
			return nil, "twilio", err
		}
		return t, "twilio", nil
	}
	return nil, provider, fmt.Errorf("SMS_PROVIDER must be gateway or twilio")
}
