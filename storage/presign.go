package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PresignUploadOptions shapes a presigned upload. What is set here is signed
// into the request, so the bucket rejects an upload that differs from it.
type PresignUploadOptions struct {
	// ContentType, when set, is the Content-Type the object is stored with
	// and the client must send.
	ContentType string

	// Size, when positive, is the exact number of bytes the client must
	// upload. Set it for uploads from clients you do not control: it is the
	// only cap a presigned PUT has.
	Size int64

	// Expiry is how long the request stays valid: [DefaultPresignExpiry]
	// when zero, at most [MaxPresignExpiry]. It has to outlive the client's
	// wait before it starts the upload, not the upload itself.
	Expiry time.Duration
}

// PresignedRequest is an HTTP request someone without credentials can send.
type PresignedRequest struct {
	Method string
	URL    string

	// Headers are part of the signature: the client must send each one with
	// this exact value. Host is left out, since every HTTP client sets it
	// from the URL.
	Headers map[string]string

	ExpiresAt time.Time
}

func (s *fileStorage) PresignUpload(ctx context.Context, key string, opts PresignUploadOptions) (*PresignedRequest, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	expiry, err := presignExpiry(opts.Expiry)
	if err != nil {
		return nil, err
	}

	in := &s3.PutObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(key),
	}
	if opts.ContentType != "" {
		in.ContentType = aws.String(opts.ContentType)
	}
	if opts.Size > 0 {
		in.ContentLength = aws.Int64(opts.Size)
	}

	out, err := s.presigner.PresignPutObject(ctx, in, func(o *s3.PresignOptions) { o.Expires = expiry })
	if err != nil {
		return nil, fmt.Errorf("storage: presign upload %q: %w", key, err)
	}

	headers := make(map[string]string, len(out.SignedHeader))
	for name, values := range out.SignedHeader {
		if strings.EqualFold(name, "Host") || len(values) == 0 {
			continue
		}
		headers[name] = values[0]
	}

	return &PresignedRequest{
		Method:    out.Method,
		URL:       out.URL,
		Headers:   headers,
		ExpiresAt: time.Now().Add(expiry),
	}, nil
}

// presignExpiry applies the default to a missing expiry and rejects one a
// presigned URL cannot carry.
func presignExpiry(expiry time.Duration) (time.Duration, error) {
	switch {
	case expiry <= 0:
		return DefaultPresignExpiry, nil
	case expiry > MaxPresignExpiry:
		return 0, fmt.Errorf("storage: requested expiry %v exceeds the %v a presigned URL can carry", expiry, MaxPresignExpiry)
	}
	return expiry, nil
}
