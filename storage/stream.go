package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// StreamPartSize is how much of a stream [FileStorage.UploadStream] reads and
// sends at a time, and so how much memory one upload holds.
const StreamPartSize = 16 << 20

// maxParts is the most parts S3 accepts in one multipart upload, which caps
// a stream at maxParts * StreamPartSize (about 156 GiB).
const maxParts = 10000

// abortTimeout bounds discarding the parts of a failed stream upload.
const abortTimeout = 30 * time.Second

func (s *fileStorage) UploadStream(ctx context.Context, key string, reader io.Reader, contentType string) (*FileResult, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	part := make([]byte, StreamPartSize)
	n, err := io.ReadFull(reader, part)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		// The whole stream fits in one part: a plain upload, with its size
		// now known.
		return s.Upload(ctx, key, bytes.NewReader(part[:n]), contentType, int64(n))
	}
	if err != nil {
		return nil, fmt.Errorf("storage: read stream for %q: %w", key, err)
	}

	created, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.config.BucketName),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: start stream upload %q: %w", key, err)
	}

	size, completed, err := s.uploadParts(ctx, key, created.UploadId, reader, part)
	if err != nil {
		// Parts of an upload that is never completed are stored, and billed,
		// until it is aborted. The caller's context may be the reason for
		// the failure, so the abort does not inherit its cancellation.
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), abortTimeout)
		defer cancel()
		if _, abortErr := s.client.AbortMultipartUpload(abortCtx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(s.config.BucketName),
			Key:      aws.String(key),
			UploadId: created.UploadId,
		}); abortErr != nil {
			err = errors.Join(err, fmt.Errorf("storage: discard the parts of %q: %w", key, abortErr))
		}
		return nil, err
	}

	result := &FileResult{Key: key, Size: size, ContentType: contentType, ETag: unquoteETag(completed.ETag)}
	if s.config.UsePublicURL {
		result.URL = s.publicURL(key)
	}
	return result, nil
}

// uploadParts sends first, a full part already read, and then the rest of
// reader part by part, and completes the upload.
func (s *fileStorage) uploadParts(ctx context.Context, key string, uploadID *string, reader io.Reader, first []byte) (int64, *s3.CompleteMultipartUploadOutput, error) {
	var (
		parts []types.CompletedPart
		size  int64
		part  = first
		n     = len(first)
	)

	for number := int32(1); ; number++ {
		if number > maxParts {
			return 0, nil, fmt.Errorf("storage: stream for %q is over the %d parts of %d bytes an upload can hold", key, maxParts, StreamPartSize)
		}

		out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:        aws.String(s.config.BucketName),
			Key:           aws.String(key),
			UploadId:      uploadID,
			PartNumber:    aws.Int32(number),
			Body:          bytes.NewReader(part[:n]),
			ContentLength: aws.Int64(int64(n)),
		})
		if err != nil {
			return 0, nil, fmt.Errorf("storage: upload part %d of %q: %w", number, key, err)
		}
		parts = append(parts, types.CompletedPart{ETag: out.ETag, PartNumber: aws.Int32(number)})
		size += int64(n)

		n, err = io.ReadFull(reader, part)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, fmt.Errorf("storage: read stream for %q: %w", key, err)
		}
		// io.ErrUnexpectedEOF: a last, shorter part, sent by the next turn,
		// whose own read then reports io.EOF.
	}

	completed, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(s.config.BucketName),
		Key:             aws.String(key),
		UploadId:        uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("storage: complete stream upload %q: %w", key, err)
	}

	return size, completed, nil
}
