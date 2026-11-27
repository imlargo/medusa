package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (f *fakeS3) HeadObject(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if f.headObjectErr != nil {
		return nil, f.headObjectErr
	}
	return f.headObjectOut, nil
}

func (f *fakeS3) CopyObject(_ context.Context, in *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	f.copyIn = in
	return &s3.CopyObjectOutput{}, nil
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.listIn = in
	if f.listOut != nil {
		return f.listOut, nil
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (f *fakeS3) CreateMultipartUpload(_ context.Context, in *s3.CreateMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	f.createIn = in
	return &s3.CreateMultipartUploadOutput{UploadId: aws.String("upload-1")}, nil
}

func (f *fakeS3) UploadPart(_ context.Context, in *s3.UploadPartInput, _ ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	if f.failPart != 0 && aws.ToInt32(in.PartNumber) == f.failPart {
		return nil, errors.New("connection reset")
	}
	n, _ := io.Copy(io.Discard, in.Body)
	f.partSizes = append(f.partSizes, n)
	return &s3.UploadPartOutput{ETag: aws.String(`"part"`)}, nil
}

func (f *fakeS3) CompleteMultipartUpload(_ context.Context, in *s3.CompleteMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	f.completeIn = in
	return &s3.CompleteMultipartUploadOutput{ETag: aws.String(`"whole-2"`)}, nil
}

func (f *fakeS3) AbortMultipartUpload(_ context.Context, in *s3.AbortMultipartUploadInput, _ ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	f.abortedUpload = aws.ToString(in.UploadId)
	return &s3.AbortMultipartUploadOutput{}, nil
}

func (f *fakePresigner) PresignPutObject(_ context.Context, in *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4signer.PresignedHTTPRequest, error) {
	var opts s3.PresignOptions
	for _, fn := range optFns {
		fn(&opts)
	}
	f.putIn = in
	f.gotExpiry = opts.Expires
	if f.err != nil {
		return nil, f.err
	}
	return &v4signer.PresignedHTTPRequest{
		Method: http.MethodPut,
		URL:    "https://example.com/presigned-put",
		SignedHeader: http.Header{
			"Host":         {"example.com"},
			"Content-Type": {aws.ToString(in.ContentType)},
		},
	}, nil
}

func TestPresignUploadReturnsTheRequestTheClientMustSend(t *testing.T) {
	presigner := &fakePresigner{}
	fs := newTestStorage(&fakeS3{}, presigner, Config{})

	req, err := fs.PresignUpload(context.Background(), "renders/42.mp4", PresignUploadOptions{
		ContentType: "video/mp4",
		Size:        1024,
		Expiry:      time.Hour,
	})
	if err != nil {
		t.Fatalf("PresignUpload() = %v", err)
	}

	if req.Method != http.MethodPut || req.URL == "" {
		t.Errorf("request = %s %q, want a PUT with a URL", req.Method, req.URL)
	}
	if got := req.Headers["Content-Type"]; got != "video/mp4" {
		t.Errorf("Headers[Content-Type] = %q, want video/mp4", got)
	}
	if _, ok := req.Headers["Host"]; ok {
		t.Error("Headers carry Host, want it left to the HTTP client")
	}
	if until := time.Until(req.ExpiresAt); until < 59*time.Minute || until > time.Hour {
		t.Errorf("ExpiresAt is in %s, want about an hour", until)
	}
	if got := aws.ToInt64(presigner.putIn.ContentLength); got != 1024 {
		t.Errorf("signed size = %d, want 1024", got)
	}
	if presigner.gotExpiry != time.Hour {
		t.Errorf("signed expiry = %s, want 1h", presigner.gotExpiry)
	}
}

// The size and the content type have to be part of the signature, or a
// client could upload something else with the same URL. This runs the real
// SDK signer, which works offline.
func TestPresignUploadSignsTheSizeAndTheContentType(t *testing.T) {
	cfg := Config{BucketName: "bucket", AccountID: "acct", AccessKeyID: "k", SecretAccessKey: "s"}
	client, err := newR2Client(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newR2Client() = %v", err)
	}
	fs := newFileStorage(&fakeS3{}, s3.NewPresignClient(client), ProviderR2, cfg)

	req, err := fs.PresignUpload(context.Background(), "uploads/a.png", PresignUploadOptions{ContentType: "image/png", Size: 2048})
	if err != nil {
		t.Fatalf("PresignUpload() = %v", err)
	}

	signed := strings.ToLower(req.URL)
	for _, header := range []string{"content-length", "content-type"} {
		if !strings.Contains(signed, header) {
			t.Errorf("URL %q does not sign %s", req.URL, header)
		}
	}
	if req.Headers["Content-Type"] != "image/png" || req.Headers["Content-Length"] != "2048" {
		t.Errorf("Headers = %v, want the content type and the length the client must send", req.Headers)
	}
}

func TestPresignUploadRejectsExpiryTooLong(t *testing.T) {
	fs := newTestStorage(&fakeS3{}, &fakePresigner{}, Config{})

	if _, err := fs.PresignUpload(context.Background(), "a.txt", PresignUploadOptions{Expiry: MaxPresignExpiry + time.Hour}); err == nil {
		t.Error("PresignUpload() = nil, want an error past the longest expiry")
	}
}

func TestHeadReturnsTheMetadata(t *testing.T) {
	modified := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	fs := newTestStorage(&fakeS3{headObjectOut: &s3.HeadObjectOutput{
		ContentLength: aws.Int64(42),
		ContentType:   aws.String("image/png"),
		ETag:          aws.String(`"abc"`),
		LastModified:  aws.Time(modified),
	}}, &fakePresigner{}, Config{})

	info, err := fs.Head(context.Background(), "a.png")
	if err != nil {
		t.Fatalf("Head() = %v", err)
	}
	want := ObjectInfo{Key: "a.png", Size: 42, ETag: "abc", LastModified: modified, ContentType: "image/png"}
	if *info != want {
		t.Errorf("Head() = %+v, want %+v", *info, want)
	}
}

// A missing object is an answer, not a failure; anything else must not be
// read as "it does not exist".
func TestExistsTellsMissingFromFailing(t *testing.T) {
	missing := newTestStorage(&fakeS3{headObjectErr: &types.NotFound{}}, &fakePresigner{}, Config{})
	if exists, err := missing.Exists(context.Background(), "a.png"); exists || err != nil {
		t.Errorf("Exists(missing) = %v, %v, want false, nil", exists, err)
	}

	failing := newTestStorage(&fakeS3{headObjectErr: errors.New("timeout")}, &fakePresigner{}, Config{})
	if exists, err := failing.Exists(context.Background(), "a.png"); exists || err == nil {
		t.Errorf("Exists(failing) = %v, %v, want false and the error", exists, err)
	}

	present := newTestStorage(&fakeS3{headObjectOut: &s3.HeadObjectOutput{}}, &fakePresigner{}, Config{})
	if exists, err := present.Exists(context.Background(), "a.png"); !exists || err != nil {
		t.Errorf("Exists(present) = %v, %v, want true, nil", exists, err)
	}
}

func TestCopyEscapesTheSourceKey(t *testing.T) {
	client := &fakeS3{}
	fs := newTestStorage(client, &fakePresigner{}, Config{})

	if err := fs.Copy(context.Background(), "tmp/my file#1.png", "final/1.png"); err != nil {
		t.Fatalf("Copy() = %v", err)
	}
	if got, want := aws.ToString(client.copyIn.CopySource), "bucket/tmp/my%20file%231.png"; got != want {
		t.Errorf("CopySource = %q, want %q", got, want)
	}
	if got := aws.ToString(client.copyIn.Key); got != "final/1.png" {
		t.Errorf("destination = %q, want final/1.png", got)
	}

	if err := fs.Copy(context.Background(), "a.png", "../b.png"); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("Copy() to an invalid key = %v, want ErrInvalidKey", err)
	}
}

func TestListPagesByPrefix(t *testing.T) {
	client := &fakeS3{listOut: &s3.ListObjectsV2Output{
		Contents: []types.Object{
			{Key: aws.String("renders/1.mp4"), Size: aws.Int64(10), ETag: aws.String(`"e1"`)},
			{Key: aws.String("renders/2.mp4"), Size: aws.Int64(20)},
		},
		NextContinuationToken: aws.String("next"),
	}}
	fs := newTestStorage(client, &fakePresigner{}, Config{})

	page, err := fs.List(context.Background(), ListOptions{Prefix: "renders/", Cursor: "prev", Limit: 2})
	if err != nil {
		t.Fatalf("List() = %v", err)
	}

	if len(page.Objects) != 2 || page.Objects[0].Key != "renders/1.mp4" || page.Objects[0].ETag != "e1" || page.Objects[1].Size != 20 {
		t.Errorf("Objects = %+v", page.Objects)
	}
	if page.NextCursor != "next" {
		t.Errorf("NextCursor = %q, want next", page.NextCursor)
	}
	in := client.listIn
	if aws.ToString(in.Prefix) != "renders/" || aws.ToString(in.ContinuationToken) != "prev" || aws.ToInt32(in.MaxKeys) != 2 {
		t.Errorf("request = prefix %q, cursor %q, limit %d", aws.ToString(in.Prefix), aws.ToString(in.ContinuationToken), aws.ToInt32(in.MaxKeys))
	}
}

func TestListValidatesItsOptions(t *testing.T) {
	fs := newTestStorage(&fakeS3{}, &fakePresigner{}, Config{})

	if _, err := fs.List(context.Background(), ListOptions{Prefix: "../secrets/"}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("List() with a bad prefix = %v, want ErrInvalidKey", err)
	}
	if _, err := fs.List(context.Background(), ListOptions{Limit: MaxListLimit + 1}); err == nil {
		t.Error("List() over the limit = nil, want an error")
	}
	if _, err := fs.List(context.Background(), ListOptions{}); err != nil {
		t.Errorf("List() of the whole bucket = %v, want nil", err)
	}
}

// A stream that fits in one part costs one request, like Upload.
func TestUploadStreamOfASmallStreamIsOnePut(t *testing.T) {
	client := &fakeS3{}
	fs := newTestStorage(client, &fakePresigner{}, Config{})

	result, err := fs.UploadStream(context.Background(), "a.txt", strings.NewReader("hello"), "text/plain")
	if err != nil {
		t.Fatalf("UploadStream() = %v", err)
	}
	if result.Size != 5 || aws.ToInt64(client.putIn.ContentLength) != 5 {
		t.Errorf("size = %d (sent %d), want 5", result.Size, aws.ToInt64(client.putIn.ContentLength))
	}
	if client.createIn != nil {
		t.Error("a multipart upload was started for a stream under one part")
	}
}

func TestUploadStreamSendsALargeStreamInParts(t *testing.T) {
	client := &fakeS3{}
	fs := newTestStorage(client, &fakePresigner{}, Config{})

	const tail = 100
	stream := io.LimitReader(zeroes{}, 2*StreamPartSize+tail)

	result, err := fs.UploadStream(context.Background(), "big.bin", stream, "application/octet-stream")
	if err != nil {
		t.Fatalf("UploadStream() = %v", err)
	}

	if want := []int64{StreamPartSize, StreamPartSize, tail}; !slices.Equal(client.partSizes, want) {
		t.Errorf("part sizes = %v, want %v", client.partSizes, want)
	}
	if result.Size != 2*StreamPartSize+tail || result.ETag != "whole-2" {
		t.Errorf("result = %+v, want the whole size and the completed ETag", result)
	}
	parts := client.completeIn.MultipartUpload.Parts
	if len(parts) != 3 || aws.ToInt32(parts[2].PartNumber) != 3 {
		t.Errorf("completed parts = %d, want 3 numbered from 1", len(parts))
	}
	if client.abortedUpload != "" {
		t.Error("a successful upload was aborted")
	}
}

// Parts of an upload nobody completes are stored and billed until aborted.
func TestUploadStreamDiscardsItsPartsWhenItFails(t *testing.T) {
	client := &fakeS3{failPart: 2}
	fs := newTestStorage(client, &fakePresigner{}, Config{})

	_, err := fs.UploadStream(context.Background(), "big.bin", io.LimitReader(zeroes{}, 2*StreamPartSize), "application/octet-stream")
	if err == nil {
		t.Fatal("UploadStream() = nil, want the part failure")
	}
	if client.abortedUpload != "upload-1" {
		t.Errorf("aborted upload = %q, want upload-1", client.abortedUpload)
	}
	if client.completeIn != nil {
		t.Error("a failed upload was completed")
	}
}

// zeroes is an endless stream of zero bytes.
type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
