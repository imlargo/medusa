// Package webhook signs and verifies messages one service sends another over
// HTTP: webhooks to and from third parties, and callbacks between services
// of the same system.
//
// It implements the Standard Webhooks scheme (standardwebhooks.com), the one
// Svix, Resend and others use. A message carries three headers:
//
//	webhook-id         a unique ID, the same on every retry
//	webhook-timestamp  when it was sent, in Unix seconds
//	webhook-signature  "v1," + base64(HMAC-SHA256(secret, id.timestamp.body))
//
// The timestamp is signed, and a message older than the verifier's tolerance
// is refused, so a captured message cannot be replayed later. The ID lets
// the receiver process each message once however many times it is delivered.
//
// Sending:
//
//	signer, err := webhook.NewSigner(secret)
//	req, err := signer.NewRequest(ctx, url, eventID, body)
//	resp, err := client.Do(req)
//
// Receiving:
//
//	verifier, err := webhook.NewVerifier(secret)
//	body, id, err := verifier.Verify(r) // r.Body is read, capped and checked
//
// Store or enqueue the message before answering, keyed by its ID, and do the
// work afterwards: the sender gets a fast 2xx, a failure in the work is
// retried from what was stored, and a redelivery is recognized by its ID.
//
// A provider with a scheme of its own, an HMAC of the body in one header, is
// verified with [HMAC].
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/internal/reqid"
)

// The headers of a Standard Webhooks message.
const (
	HeaderID        = "webhook-id"
	HeaderTimestamp = "webhook-timestamp"
	HeaderSignature = "webhook-signature"
)

const (
	// DefaultTolerance is how far a message's timestamp may be from now, in
	// either direction, before it is refused.
	DefaultTolerance = 5 * time.Minute

	// DefaultMaxBody caps the body a verifier reads.
	DefaultMaxBody = 1 << 20

	// secretPrefix marks a secret as base64, as Standard Webhooks writes them.
	secretPrefix = "whsec_"

	// signatureVersion is the scheme this package signs with.
	signatureVersion = "v1"

	// minSecretBytes is the shortest secret accepted: the spec's lower bound.
	minSecretBytes = 24

	// requestIDHeader carries the sender's request ID to the receiver.
	requestIDHeader = "X-Request-ID"
)

// ErrInvalidSignature: the message is not signed with a known secret, its
// timestamp is outside the tolerance, or a header is missing. The reason is
// its cause, for the log; the sender is told only that it was refused.
var ErrInvalidSignature = apperr.Unauthenticated("webhook.invalid_signature", "the message's signature is not valid")

// ParseSecret reads a secret: "whsec_" followed by base64, as Standard
// Webhooks writes them, or any other string, used as its raw bytes. It
// refuses a secret under 24 bytes.
func ParseSecret(secret string) ([]byte, error) {
	key := []byte(secret)
	if encoded, ok := strings.CutPrefix(secret, secretPrefix); ok {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("webhook: the secret after whsec_ is not base64")
		}
		key = decoded
	}
	if len(key) < minSecretBytes {
		return nil, fmt.Errorf("webhook: the secret has %d bytes, the minimum is %d", len(key), minSecretBytes)
	}
	return key, nil
}

// Signer signs outgoing messages with one secret.
type Signer struct {
	key []byte
}

// NewSigner signs with secret (see [ParseSecret]).
func NewSigner(secret string) (*Signer, error) {
	key, err := ParseSecret(secret)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key}, nil
}

// Headers returns the three headers of a message with this id and body, sent
// at the given time. Use the same id on every retry of one message.
func (s *Signer) Headers(id string, at time.Time, body []byte) http.Header {
	timestamp := strconv.FormatInt(at.Unix(), 10)

	header := http.Header{}
	header.Set(HeaderID, id)
	header.Set(HeaderTimestamp, timestamp)
	header.Set(HeaderSignature, signatureVersion+","+sign(s.key, id, timestamp, body))
	return header
}

// NewRequest builds the POST that delivers a JSON body to url, signed now.
// It carries the request ID of ctx, when there is one, so the receiver's
// logs join the sender's.
func (s *Signer) NewRequest(ctx context.Context, url, id string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webhook: build request: %w", err)
	}

	for name, values := range s.Headers(id, time.Now(), body) {
		req.Header[name] = values
	}
	req.Header.Set("Content-Type", "application/json")
	if id := reqid.From(ctx); id != "" {
		req.Header.Set(requestIDHeader, id)
	}
	return req, nil
}

// Verifier checks incoming messages against one or more secrets.
type Verifier struct {
	keys      [][]byte
	tolerance time.Duration
	maxBody   int64
	now       func() time.Time
}

// VerifierOption configures a Verifier.
type VerifierOption func(*Verifier)

// WithTolerance sets how far a message's timestamp may be from now.
func WithTolerance(d time.Duration) VerifierOption {
	return func(v *Verifier) { v.tolerance = d }
}

// WithMaxBody caps the body Verify reads.
func WithMaxBody(n int64) VerifierOption {
	return func(v *Verifier) { v.maxBody = n }
}

// WithSecrets adds secrets a message may also be signed with. It is how a
// secret is rotated: accept the old and the new one until every sender has
// the new one.
func WithSecrets(secrets ...string) VerifierOption {
	return func(v *Verifier) {
		for _, secret := range secrets {
			key, err := ParseSecret(secret)
			if err != nil {
				panic(err.Error())
			}
			v.keys = append(v.keys, key)
		}
	}
}

// NewVerifier accepts messages signed with secret (see [ParseSecret]).
func NewVerifier(secret string, opts ...VerifierOption) (*Verifier, error) {
	key, err := ParseSecret(secret)
	if err != nil {
		return nil, err
	}

	v := &Verifier{keys: [][]byte{key}, tolerance: DefaultTolerance, maxBody: DefaultMaxBody, now: time.Now}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
}

// Verify reads r's body and checks the message's signature and age. It
// returns the body and the message's ID. Anything wrong with the message is
// [ErrInvalidSignature]; a body over the cap is *http.MaxBytesError.
func (v *Verifier) Verify(r *http.Request) (body []byte, id string, err error) {
	body, err = readBody(r, v.maxBody)
	if err != nil {
		return nil, "", err
	}
	if err := v.VerifyMessage(r.Header, body); err != nil {
		return nil, "", err
	}
	return body, r.Header.Get(HeaderID), nil
}

// VerifyMessage checks a message whose body was already read.
func (v *Verifier) VerifyMessage(header http.Header, body []byte) error {
	id, timestamp, signatures := header.Get(HeaderID), header.Get(HeaderTimestamp), header.Get(HeaderSignature)
	if id == "" || timestamp == "" || signatures == "" {
		return ErrInvalidSignature.WithCause(errors.New("a webhook-id, webhook-timestamp or webhook-signature header is missing"))
	}

	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidSignature.WithCause(errors.New("webhook-timestamp is not a Unix time"))
	}
	if age := v.now().Sub(time.Unix(seconds, 0)); age > v.tolerance || age < -v.tolerance {
		return ErrInvalidSignature.WithCause(fmt.Errorf("the message's timestamp is %s from now, over the %s tolerance", age.Round(time.Second), v.tolerance))
	}

	// The header may carry several signatures, separated by spaces: one per
	// secret the sender is rotating through.
	for candidate := range strings.FieldsSeq(signatures) {
		version, signature, ok := strings.Cut(candidate, ",")
		if !ok || version != signatureVersion {
			continue
		}
		for _, key := range v.keys {
			if hmac.Equal([]byte(signature), []byte(sign(key, id, timestamp, body))) {
				return nil
			}
		}
	}
	return ErrInvalidSignature.WithCause(errors.New("no signature matches a known secret"))
}

// sign is the Standard Webhooks signature: base64(HMAC-SHA256(key, id.timestamp.body)).
func sign(key []byte, id, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Encoding is how a provider writes a signature's bytes.
type Encoding int

const (
	// Hex is lowercase hexadecimal, the most common.
	Hex Encoding = iota

	// Base64 is standard base64.
	Base64
)

// HMAC verifies messages from a provider with a scheme of its own: an
// HMAC-SHA256 of the raw body under a shared secret, written in one header.
// GitHub ("X-Hub-Signature-256", prefix "sha256="), Meta and cal.com work
// this way.
//
// It does not protect against replay: the body carries no signed time. Use
// the provider's event ID to process each message once.
type HMAC struct {
	// Header is the name of the header that carries the signature.
	Header string

	// Prefix is what precedes the signature in the header, such as
	// "sha256=". Empty when there is none.
	Prefix string

	// Encoding is how the signature is written.
	Encoding Encoding

	// Secret is the shared secret, used as its raw bytes.
	Secret string

	// MaxBody caps the body Verify reads, [DefaultMaxBody] when zero.
	MaxBody int64
}

// Verify reads r's body and checks its signature. It returns the body.
// A missing or wrong signature is [ErrInvalidSignature].
func (h HMAC) Verify(r *http.Request) ([]byte, error) {
	maxBody := h.MaxBody
	if maxBody == 0 {
		maxBody = DefaultMaxBody
	}
	body, err := readBody(r, maxBody)
	if err != nil {
		return nil, err
	}

	got, ok := strings.CutPrefix(r.Header.Get(h.Header), h.Prefix)
	if !ok || got == "" {
		return nil, ErrInvalidSignature.WithCause(fmt.Errorf("the %s header is missing or malformed", h.Header))
	}

	mac := hmac.New(sha256.New, []byte(h.Secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if h.Encoding == Base64 {
		want = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	} else {
		got = strings.ToLower(got)
	}

	if !hmac.Equal([]byte(got), []byte(want)) {
		return nil, ErrInvalidSignature.WithCause(fmt.Errorf("the %s header does not match the body", h.Header))
	}
	return body, nil
}

// readBody reads at most limit bytes of r's body and leaves r.Body readable
// again, so a later binder sees the same bytes.
func readBody(r *http.Request, limit int64) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("webhook: read body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}
