#!/bin/sh
set -eu

: "${PHONE:?set PHONE}"
: "${OTP_CODE:?set OTP_CODE}"
: "${WIDGET_RECORD_ID:?set WIDGET_RECORD_ID}"
: "${CAPTCHA_TOKEN:?set CAPTCHA_TOKEN}"

curl --fail-with-body --request POST http://localhost:8080/orders/login \
  --header 'Content-Type: application/json' \
  --data "{\"phone\":\"$PHONE\",\"code\":\"$OTP_CODE\",\"widget_record_id\":\"$WIDGET_RECORD_ID\",\"captcha_token\":\"$CAPTCHA_TOKEN\",\"order_id\":\"ord_1042\"}"
