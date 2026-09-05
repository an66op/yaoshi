package user

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"backend/services"

	"github.com/gin-gonic/gin"
)

func paymentAccountMultipartContext(t *testing.T, filename string, imageData []byte) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for field, value := range map[string]string{
		"account_type": "wechat", "label": "常用微信", "account_name": "测试用户", "account_no": "wx-test", "is_default": "true",
	} {
		if err := writer.WriteField(field, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := writer.CreateFormFile("qr_code", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(imageData); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/api/member/payment-accounts", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return context
}

func validPaymentQRCodePNG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 40, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, canvas); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestBindMemberPaymentAccountIgnoresOriginalFilenameAndSanitizesContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context := paymentAccountMultipartContext(t, "../../invoice.svg", validPaymentQRCodePNG(t))
	request, qrCode, err := bindMemberPaymentAccount(context)
	if err != nil {
		t.Fatal(err)
	}
	if qrCode == nil {
		t.Fatal("valid raster content was not sanitized")
	}
	if request.AccountType != "wechat" || request.AccountNo != "wx-test" || !request.IsDefault {
		t.Fatalf("multipart account fields = %#v", request)
	}
}

func TestBindMemberPaymentAccountKeepsLegacyJSONCreateCompatible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/api/member/payment-accounts", bytes.NewBufferString(`{"account_type":"bank","account_name":"工资卡","account_no":"62220001"}`))
	context.Request.Header.Set("Content-Type", "application/json")

	request, qrCode, err := bindMemberPaymentAccount(context)
	if err != nil {
		t.Fatal(err)
	}
	if qrCode != nil || request.AccountType != "bank" || request.AccountNo != "62220001" {
		t.Fatalf("legacy JSON create changed: request=%#v qr=%#v", request, qrCode)
	}
}

func TestBindMemberPaymentAccountRejectsScriptDisguisedAsImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context := paymentAccountMultipartContext(t, "payment.png", []byte(`<svg><script>alert(1)</script></svg>`))
	if _, _, err := bindMemberPaymentAccount(context); err == nil {
		t.Fatal("script upload was accepted")
	}
}

func TestPaymentAccountQRCodeRequiresAuthenticatedMemberBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "12"}}

	(&memberHandler{}).PaymentAccountQRCode(context)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated QR read status = %d, want 401", recorder.Code)
	}
}

func TestPaymentAccountQRCodeStreamsWithPrivateSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := validPaymentQRCodePNG(t)
	target := filepath.Join(t.TempDir(), "qr.png")
	if err := os.WriteFile(target, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	streamPaymentQRCode(context, &services.MemberPaymentQRCode{File: file, Size: int64(len(payload))})

	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), payload) {
		t.Fatalf("streamed QR response status/bytes = %d/%d", recorder.Code, recorder.Body.Len())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q", got)
	}
}
