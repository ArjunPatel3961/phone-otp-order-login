package otpcheckout

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type verifierStub struct {
	captchaErr error
	phoneErr   error
	calls      []string
}

func (v *verifierStub) VerifyCaptcha(context.Context, string, string, string) error {
	v.calls = append(v.calls, "captcha")
	return v.captchaErr
}

func (v *verifierStub) VerifyPhone(context.Context, string, string) error {
	v.calls = append(v.calls, "phone")
	return v.phoneErr
}

func TestOrderLoginDecision(t *testing.T) {
	rejected := errors.New("verification rejected")
	tests := []struct {
		name       string
		verifier   *verifierStub
		wantStages []string
		wantCalls  []string
		wantErr    error
	}{
		{
			name:       "verified customer sees the order lifecycle",
			verifier:   &verifierStub{},
			wantStages: []string{"checkout", "fulfillment", "receipt", "customer_update"},
			wantCalls:  []string{"captcha", "phone"},
		},
		{
			name:      "captcha rejection stops before phone verification",
			verifier:  &verifierStub{captchaErr: rejected},
			wantCalls: []string{"captcha"},
			wantErr:   rejected,
		},
		{
			name:      "phone rejection exposes no order state",
			verifier:  &verifierStub{phoneErr: rejected},
			wantCalls: []string{"captcha", "phone"},
			wantErr:   rejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewOrderLogin(tt.verifier).Authorize(context.Background(), LoginOrderInput{
				Phone: "+15555550123", Code: "123456", WidgetRecordID: "widget-record", CaptchaToken: "captcha-token", OrderID: "ord_1042",
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authorize() error = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(tt.verifier.calls, tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", tt.verifier.calls, tt.wantCalls)
			}
			stages := make([]string, 0, len(result.Updates))
			for _, update := range result.Updates {
				stages = append(stages, update.Stage)
			}
			if !slices.Equal(stages, tt.wantStages) {
				t.Fatalf("stages = %v, want %v", stages, tt.wantStages)
			}
		})
	}
}
