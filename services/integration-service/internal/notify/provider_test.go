package notify

import "testing"

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

var (
	gatewayEnv = map[string]string{"GATEWAY_URL": "https://gateway.example.test", "GATEWAY_API_KEY": "cgk_0123456789abcdef"}
	twilioEnv  = map[string]string{"TWILIO_ACCOUNT_SID": "AC0123456789abcdef0123456789abcdef", "TWILIO_AUTH_TOKEN": "token", "TWILIO_FROM": "+94770000000"}
)

func merge(parts ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, p := range parts {
		for k, v := range p {
			out[k] = v
		}
	}
	return out
}

func TestProviderFromEnv(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"nothing configured is an error, not a silent no-op", map[string]string{}, "twilio", true},
		{"the gateway when only it is configured", gatewayEnv, "gateway", false},
		{"twilio when only it is configured", twilioEnv, "twilio", false},
		{"the gateway wins when both are configured", merge(gatewayEnv, twilioEnv), "gateway", false},
		{"forcing twilio with both configured", merge(gatewayEnv, twilioEnv, map[string]string{"SMS_PROVIDER": "twilio"}), "twilio", false},
		{"forcing the gateway with both configured", merge(gatewayEnv, twilioEnv, map[string]string{"SMS_PROVIDER": " Gateway "}), "gateway", false},
		{"a forced gateway with a bad key does not fall back to twilio", merge(twilioEnv, map[string]string{"SMS_PROVIDER": "gateway", "GATEWAY_URL": "https://gateway.example.test", "GATEWAY_API_KEY": "nope"}), "gateway", true},
		{"a half-configured gateway does not fall back to twilio", merge(twilioEnv, map[string]string{"GATEWAY_URL": "https://gateway.example.test"}), "gateway", true},
		{"a forced twilio that is empty does not fall back to the gateway", merge(gatewayEnv, map[string]string{"SMS_PROVIDER": "twilio"}), "twilio", true},
		{"an unknown provider name", merge(gatewayEnv, map[string]string{"SMS_PROVIDER": "carrier-pigeon"}), "carrier-pigeon", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sender, name, err := ProviderFromEnv(env(c.env))
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, c.wantErr)
			}
			if name != c.want {
				t.Fatalf("provider = %q, want %q", name, c.want)
			}
			if c.wantErr && sender != nil {
				t.Fatal("no sender must come with an error")
			}
			if !c.wantErr && sender == nil {
				t.Fatal("a valid configuration must give a sender")
			}
		})
	}
}

func TestGatewayQueueingDefaultsOnAndCanBeSwitchedOff(t *testing.T) {
	for value, want := range map[string]bool{"": true, "true": true, "false": false, " FALSE ": false} {
		sender, _, err := ProviderFromEnv(env(merge(gatewayEnv, map[string]string{"GATEWAY_QUEUE": value})))
		if err != nil {
			t.Fatal(err)
		}
		if got := sender.(*Gateway).config.Queue; got != want {
			t.Errorf("GATEWAY_QUEUE=%q -> queue %v, want %v", value, got, want)
		}
	}
}
