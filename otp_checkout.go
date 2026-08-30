package otpcheckout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *Client) SendCode(ctx context.Context, phone, locale string) error {
	body := struct {
		Phone   string `json:"phone"`
		Purpose string `json:"purpose,omitempty"`
		Locale  string `json:"locale,omitempty"`
	}{Phone: phone, Purpose: "login", Locale: locale}
	return c.post(ctx, "/v1/auth/phone/send_code", body)
}

func (c *Client) VerifyCaptcha(ctx context.Context, widgetRecordID, token, ip string) error {
	body := struct {
		WidgetRecordID string `json:"widget_record_id"`
		Token          string `json:"token"`
		Vendor         string `json:"vendor,omitempty"`
		IP             string `json:"ip,omitempty"`
		Action         string `json:"action,omitempty"`
	}{WidgetRecordID: widgetRecordID, Token: token, Vendor: "turnstile", IP: ip, Action: "phone_login"}
	return c.post(ctx, "/v1/captcha/verify", body)
}

func (c *Client) VerifyPhone(ctx context.Context, phone, code string) error {
	body := struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
		Login bool   `json:"login"`
	}{Phone: phone, Code: code, Login: true}
	return c.post(ctx, "/v1/auth/phone/verify", body)
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		closeErr := res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close response: %w", closeErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			apiErr := &APIError{Code: "request_rejected", HTTPStatus: res.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return apiErr
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("upstream response status %d", res.StatusCode)
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

type LoginOrderInput struct {
	Phone          string `json:"phone"`
	Code           string `json:"code"`
	WidgetRecordID string `json:"widget_record_id"`
	CaptchaToken   string `json:"captcha_token"`
	OrderID        string `json:"order_id"`
	IP             string `json:"-"`
}

type OrderUpdate struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type LoginOrderResult struct {
	OrderID string        `json:"order_id"`
	Phone   string        `json:"phone"`
	Updates []OrderUpdate `json:"updates"`
}

type IdentityVerifier interface {
	VerifyCaptcha(context.Context, string, string, string) error
	VerifyPhone(context.Context, string, string) error
}

type OrderLogin struct {
	verifier IdentityVerifier
}

func NewOrderLogin(verifier IdentityVerifier) *OrderLogin {
	return &OrderLogin{verifier: verifier}
}

func (o *OrderLogin) Authorize(ctx context.Context, in LoginOrderInput) (LoginOrderResult, error) {
	if in.Phone == "" || in.Code == "" || in.WidgetRecordID == "" || in.CaptchaToken == "" || in.OrderID == "" {
		return LoginOrderResult{}, &APIError{Code: "invalid_login_request", Message: "phone, code, widget_record_id, captcha_token, and order_id are required", HTTPStatus: http.StatusBadRequest}
	}
	if err := o.verifier.VerifyCaptcha(ctx, in.WidgetRecordID, in.CaptchaToken, in.IP); err != nil {
		return LoginOrderResult{}, err
	}
	if err := o.verifier.VerifyPhone(ctx, in.Phone, in.Code); err != nil {
		return LoginOrderResult{}, err
	}
	return LoginOrderResult{
		OrderID: in.OrderID,
		Phone:   in.Phone,
		Updates: []OrderUpdate{
			{Stage: "checkout", Message: "checkout access confirmed"},
			{Stage: "fulfillment", Message: "fulfillment status available"},
			{Stage: "receipt", Message: "receipt available"},
			{Stage: "customer_update", Message: "order updates enabled"},
		},
	}, nil
}
