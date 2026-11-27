// Package storage provides object storage for application files: upload,
// download, inspect, copy, list and delete objects, and hand a client a link
// to read one or a request to upload one.
//
// # Getting started
//
//	fs, err := storage.NewFileStorage(ctx, storage.ProviderR2, storage.Config{
//	    BucketName:      "assets",
//	    AccountID:       accountID,
//	    AccessKeyID:     accessKeyID,
//	    SecretAccessKey: secretAccessKey,
//	})
//	if err != nil {
//	    return err
//	}
//
//	result, err := fs.Upload(ctx, "avatars/42.png", reader, "image/png", size)
//
// # Uploads that skip your server
//
// A large file should not travel through the API on its way to the bucket.
// [FileStorage.PresignUpload] returns a request the client sends itself:
//
//	req, err := fs.PresignUpload(ctx, "uploads/42.mp4", storage.PresignUploadOptions{
//	    ContentType: "video/mp4",
//	    Size:        size, // signed: the bucket rejects any other length
//	})
//	// hand req.Method, req.URL and req.Headers to the client
//
// When the upload is done, [FileStorage.Head] tells you what actually
// arrived. For content this process produces or fetches without knowing its
// size, [FileStorage.UploadStream] sends it in parts.
//
// # Fails at construction, not on first use
//
// [NewFileStorage] confirms the bucket is reachable with the credentials it
// was given before it returns. A typo'd bucket name or a revoked key is then
// a startup error naming the problem, rather than a failure surfacing an hour
// into production on whichever request happens to upload first.
//
// # Keys are validated before they are sent
//
// Every method that takes a key runs it through [ValidateKey] first. A key
// built from user input — a filename, a slug — is not automatically safe to
// hand to a provider: nothing in the SDK stops an application from sending an
// empty key, one with a ".." segment, or one carrying a control character
// that will corrupt a log line or a listing later. Rejecting those before a
// request is sent turns a provider-specific failure two calls away from its
// cause into [ErrInvalidKey] naming exactly what was wrong.
//
// # Deletion reports partial failure
//
// [FileStorage.BulkDelete] returns a [DeleteResult] listing which keys were
// removed and which were not, rather than one combined error. A thousand-key
// batch where three keys are denied by a bucket policy is not "an error"; it
// is 997 successes and three failures a caller can retry individually,
// and folding that into a single error string would throw the distinction
// away.
//
// # What it will not do
//
// This is not a general-purpose blob abstraction over every provider that
// speaks a different protocol: it targets S3-compatible object storage.
// Providers are added deliberately — see [Provider] — because building a
// client for one that was never tested against this package is precisely the
// kind of failure this package exists to surface at startup instead of on
// first use.
package storage
