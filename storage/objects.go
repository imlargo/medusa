package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// MaxListLimit is the most objects one [FileStorage.List] page holds. It is
// S3's own limit, and the default when [ListOptions.Limit] is not set.
const MaxListLimit = 1000

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time

	// ContentType is set by Head. A listing does not carry it.
	ContentType string
}

// ListOptions selects a page of a listing.
type ListOptions struct {
	// Prefix limits the listing to keys that start with it. End it with "/"
	// to list a "folder" without also matching its siblings ("a/" does not
	// match "ab"). Empty lists the whole bucket.
	Prefix string

	// Cursor is the NextCursor of the previous page. Empty starts over.
	Cursor string

	// Limit is the page size, [MaxListLimit] at most and by default.
	Limit int
}

// ListResult is one page of a listing.
type ListResult struct {
	Objects []ObjectInfo

	// NextCursor asks for the next page. It is empty on the last one.
	NextCursor string
}

func (s *fileStorage) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: head %q: %w", key, err)
	}

	return &ObjectInfo{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		ETag:         unquoteETag(out.ETag),
		LastModified: aws.ToTime(out.LastModified),
		ContentType:  aws.ToString(out.ContentType),
	}, nil
}

func (s *fileStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Head(ctx, key)
	if IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *fileStorage) Copy(ctx context.Context, sourceKey, destinationKey string) error {
	if err := ValidateKey(sourceKey); err != nil {
		return err
	}
	if err := ValidateKey(destinationKey); err != nil {
		return err
	}

	// CopySource is "bucket/key" with the key URL-encoded, slashes kept.
	source := s.config.BucketName + "/" + (&url.URL{Path: sourceKey}).EscapedPath()

	if _, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.config.BucketName),
		Key:        aws.String(destinationKey),
		CopySource: aws.String(source),
	}); err != nil {
		return fmt.Errorf("storage: copy %q to %q: %w", sourceKey, destinationKey, err)
	}

	return nil
}

func (s *fileStorage) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	// A prefix is a key that may stop mid-name or at a "/", so it is held to
	// the same rules once the trailing slash is set aside.
	if prefix := strings.TrimSuffix(opts.Prefix, "/"); prefix != "" {
		if err := ValidateKey(prefix); err != nil {
			return nil, err
		}
	}
	if opts.Limit < 0 || opts.Limit > MaxListLimit {
		return nil, fmt.Errorf("storage: list limit %d is outside 0-%d", opts.Limit, MaxListLimit)
	}
	limit := opts.Limit
	if limit == 0 {
		limit = MaxListLimit
	}

	in := &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.config.BucketName),
		MaxKeys: aws.Int32(int32(limit)),
	}
	if opts.Prefix != "" {
		in.Prefix = aws.String(opts.Prefix)
	}
	if opts.Cursor != "" {
		in.ContinuationToken = aws.String(opts.Cursor)
	}

	out, err := s.client.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("storage: list %q: %w", opts.Prefix, err)
	}

	result := &ListResult{
		Objects:    make([]ObjectInfo, 0, len(out.Contents)),
		NextCursor: aws.ToString(out.NextContinuationToken),
	}
	for _, object := range out.Contents {
		result.Objects = append(result.Objects, ObjectInfo{
			Key:          aws.ToString(object.Key),
			Size:         aws.ToInt64(object.Size),
			ETag:         unquoteETag(object.ETag),
			LastModified: aws.ToTime(object.LastModified),
		})
	}

	return result, nil
}

// unquoteETag strips the quotes S3 wraps an ETag in.
func unquoteETag(etag *string) string {
	return strings.Trim(aws.ToString(etag), `"`)
}
