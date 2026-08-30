package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	otpcheckout "example.com/phone-otp-order-login"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := otpcheckout.NewClient(apiKey, nil)
	orderLogin := otpcheckout.NewOrderLogin(client)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /otp/send", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Phone  string `json:"phone"`
			Locale string `json:"locale"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Phone == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phone is required"})
			return
		}
		if err := client.SendCode(r.Context(), input.Phone, input.Locale); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "code_sent"})
	})
	mux.HandleFunc("POST /orders/login", func(w http.ResponseWriter, r *http.Request) {
		var input otpcheckout.LoginOrderInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		input.IP = r.RemoteAddr
		result, err := orderLogin.Authorize(r.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("checkout auth listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var apiErr *otpcheckout.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status = apiErr.HTTPStatus
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
