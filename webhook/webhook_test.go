package webhook

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imlargo/medusa/apperr"
)

const testSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// The example of the Standard Webhooks and Svix documentation: the same
// inputs have to give the same signature here, or this is not the scheme.
func TestSignMatchesThePublishedExample(t *testing.T) {
	signer, err := NewSigner(testSecret)
	if err != nil {
		t.Fatalf("NewSigner() = %v", err)
	}

	header := signer.Headers("msg_p5jXN8AQM9LWM0D4loKWxJek", time.Unix(1614265330, 0), []byte(`{"test": 2432232314}`))

	if got, want := header.Get(HeaderSignature), "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="; got != want {
		t.Errorf("signature = %q, want %q", got, want)
	}
	if got := header.Get(HeaderTimestamp); got != "1614265330" {
		t.Errorf("timestamp = %q, want 1614265330", got)
	}
}

// signed builds a request the way a sender would.
func signed(t *testing.T, secret, id, body string) *http.Request {
	t.Helper()
	signer, err := NewSigner(secret)
	if err != nil {
		t.Fatal(err)
	}
	req, err := signer.NewRequest(t.Context(), "https://api.example.com/hooks", id, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func newVerifier(t *testing.T, opts ...VerifierOption) *Verifier {
	t.Helper()
	v, err := NewVerifier(testSecret, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVerifyAcceptsWhatTheSignerSent(t *testing.T) {
	req := signed(t, testSecret, "evt_1", `{"status":"done"}`)

	body, id, err := newVerifier(t).Verify(req)
	if err != nil {
		t.Fatalf("Verify() = %v", err)
	}
	if string(body) != `{"status":"done"}` || id != "evt_1" {
		t.Errorf("Verify() = %s, %q, want the body and evt_1", body, id)
	}

	// The body can be read again, by a binder after the check.
	again := make([]byte, len(body))
	if _, err := req.Body.Read(again); err != nil || string(again) != string(body) {
		t.Errorf("body after Verify = %q, %v, want it readable again", again, err)
	}
}

func TestVerifyRefusesWhatItCannotTrust(t *testing.T) {
	const otherSecret = "whsec_C2FVsBQIhrscChlQIMV+b5sSYspob7oD"

	tests := []struct {
		name   string
		mutate func(r *http.Request) *http.Request
	}{
		{"another secret", func(*http.Request) *http.Request { return signed(t, otherSecret, "evt_1", `{"a":1}`) }},
		{"a changed body", func(r *http.Request) *http.Request {
			changed := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":2}`))
			changed.Header = r.Header
			return changed
		}},
		{"a changed id", func(r *http.Request) *http.Request { r.Header.Set(HeaderID, "evt_2"); return r }},
		{"a changed timestamp", func(r *http.Request) *http.Request { r.Header.Set(HeaderTimestamp, "1614265330"); return r }},
		{"no signature", func(r *http.Request) *http.Request { r.Header.Del(HeaderSignature); return r }},
		{"another version", func(r *http.Request) *http.Request {
			r.Header.Set(HeaderSignature, strings.Replace(r.Header.Get(HeaderSignature), "v1,", "v2,", 1))
			return r
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.mutate(signed(t, testSecret, "evt_1", `{"a":1}`))

			_, _, err := newVerifier(t).Verify(req)
			if !errors.Is(err, ErrInvalidSignature) {
				t.Fatalf("Verify() = %v, want ErrInvalidSignature", err)
			}
			if apperr.From(err).Cause() == nil {
				t.Error("the refusal has no cause, want the reason for the log")
			}
		})
	}
}

// A captured message must not work later: the timestamp is signed, and an
// old one is refused even with a valid signature.
func TestVerifyRefusesAMessageOutsideTheTolerance(t *testing.T) {
	req := signed(t, testSecret, "evt_1", `{}`)

	late := newVerifier(t)
	late.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if _, _, err := late.Verify(req); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify() six minutes later = %v, want ErrInvalidSignature", err)
	}

	early := newVerifier(t)
	early.now = func() time.Time { return time.Now().Add(-6 * time.Minute) }
	if _, _, err := early.Verify(signed(t, testSecret, "evt_1", `{}`)); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify() of a message from the future = %v, want ErrInvalidSignature", err)
	}
}

// Rotating a secret: the receiver accepts both until every sender has moved.
func TestVerifyAcceptsAnySecretItWasGiven(t *testing.T) {
	const newSecret = "whsec_C2FVsBQIhrscChlQIMV+b5sSYspob7oD"
	verifier := newVerifier(t, WithSecrets(newSecret))

	for _, secret := range []string{testSecret, newSecret} {
		if _, _, err := verifier.Verify(signed(t, secret, "evt_1", `{}`)); err != nil {
			t.Errorf("Verify() of a message signed with %s = %v, want nil", secret[:12], err)
		}
	}
}

func TestVerifyCapsTheBody(t *testing.T) {
	req := signed(t, testSecret, "evt_1", strings.Repeat("a", 4096))

	_, _, err := newVerifier(t, WithMaxBody(1024)).Verify(req)
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); !tooLarge {
		t.Errorf("Verify() = %v, want *http.MaxBytesError", err)
	}
}

func TestParseSecret(t *testing.T) {
	if _, err := ParseSecret("short"); err == nil {
		t.Error("ParseSecret() of a short secret = nil, want an error")
	}
	if _, err := ParseSecret("whsec_not base64!!"); err == nil {
		t.Error("ParseSecret() of bad base64 = nil, want an error")
	}
	if key, err := ParseSecret(strings.Repeat("k", 32)); err != nil || len(key) != 32 {
		t.Errorf("ParseSecret() of a raw secret = %d bytes, %v, want its 32 bytes", len(key), err)
	}
}

func TestHMACVerifiesAProvidersOwnScheme(t *testing.T) {
	// GitHub's documented example: secret "It's a Secret to Everybody",
	// payload "Hello, World!".
	github := HMAC{Header: "X-Hub-Signature-256", Prefix: "sha256=", Secret: "It's a Secret to Everybody"}
	const signature = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"

	request := func(body, header string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if header != "" {
			req.Header.Set("X-Hub-Signature-256", header)
		}
		return req
	}

	body, err := github.Verify(request("Hello, World!", signature))
	if err != nil || string(body) != "Hello, World!" {
		t.Fatalf("Verify() = %q, %v, want the body", body, err)
	}

	for name, req := range map[string]*http.Request{
		"a changed body":     request("Hello, World?", signature),
		"no header":          request("Hello, World!", ""),
		"without the prefix": request("Hello, World!", strings.TrimPrefix(signature, "sha256=")),
	} {
		if _, err := github.Verify(req); !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("Verify() with %s = %v, want ErrInvalidSignature", name, err)
		}
	}
}
