// Package r2 implements Object interface for Cloudflare R2.
package r2

import (
	"bytes"
	"codesfer/pkg/object"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Config holds R2 connection details.
type Config struct {
	AccountID        string
	AccessKey        string
	SecretAccessKey  string
	Bucket           string
	Region           string
	EndpointOverride string
}

// Storage implements object.ObjectStorage for Cloudflare R2.
type Storage struct {
	client *s3.Client
	bucket string
}

// Init bootstraps the R2 client using static credentials.
func (s *Storage) Init(ctx context.Context, param any) error {
	cfg, ok := param.(Config)
	if !ok {
		if p, ok := param.(*Config); ok && p != nil {
			cfg = *p
		} else {
			return fmt.Errorf("r2: unexpected config type %T", param)
		}
	}

	if cfg.AccountID == "" && cfg.EndpointOverride == "" {
		return errors.New("r2: AccountID or EndpointOverride required")
	}
	if cfg.AccessKey == "" || cfg.SecretAccessKey == "" || cfg.Bucket == "" {
		return errors.New("r2: AccessKey, SecretAccessKey, and Bucket are required")
	}
	if cfg.Region == "" {
		cfg.Region = "auto"
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return fmt.Errorf("r2: load config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		base := cfg.EndpointOverride
		if base == "" {
			base = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
		}
		o.BaseEndpoint = aws.String(base)
	})

	s.client = client
	s.bucket = cfg.Bucket
	return nil
}

// Close cleans up resources; no-op for R2.
func (s *Storage) Close(_ context.Context) error {
	return nil
}

// Put uploads the full object body.
func (s *Storage) Put(ctx context.Context, key string, r io.Reader, sizeHint int64, contentType string, meta map[string]string) (object.Object, error) {
	if err := s.ensureClient(); err != nil {
		return object.Object{}, err
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
		Metadata: func() map[string]string {
			if meta == nil {
				return nil
			}
			c := make(map[string]string, len(meta))
			maps.Copy(c, meta)
			return c
		}(),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if sizeHint >= 0 {
		input.ContentLength = aws.Int64(sizeHint)
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return object.Object{}, mapError(err)
	}

	metaObj, err := s.Stat(ctx, key)
	if err != nil {
		return object.Object{}, err
	}
	return metaObj, nil
}

// MultipartPut streams large uploads in parts.
func (s *Storage) MultipartPut(ctx context.Context, key string, r io.Reader, partSize int64, meta map[string]string) (object.Object, error) {
	if err := s.ensureClient(); err != nil {
		return object.Object{}, err
	}

	createResp, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		Metadata: meta,
	})
	if err != nil {
		return object.Object{}, mapError(err)
	}

	uploadID := aws.ToString(createResp.UploadId)
	var completedParts []types.CompletedPart
	buf := make([]byte, partSize)
	partNum := int32(1)

	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			partResp, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
				Bucket:     aws.String(s.bucket),
				Key:        aws.String(key),
				UploadId:   aws.String(uploadID),
				PartNumber: aws.Int32(partNum),
				Body:       bytes.NewReader(buf[:n]),
			})
			if err != nil {
				return object.Object{}, mapError(err)
			}

			completedParts = append(completedParts, types.CompletedPart{
				ETag:       partResp.ETag,
				PartNumber: aws.Int32(partNum),
			})
			partNum++
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return object.Object{}, fmt.Errorf("r2: read multipart chunk: %w", readErr)
		}
	}

	if _, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}); err != nil {
		return object.Object{}, mapError(err)
	}

	return s.Stat(ctx, key)
}

// Get fetches metadata plus a streaming reader.
func (s *Storage) Get(ctx context.Context, key string, rng *object.Range) (object.Object, io.ReadCloser, error) {
	if err := s.ensureClient(); err != nil {
		return object.Object{}, nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if rng != nil {
		input.Range = aws.String(rangeHeader(*rng))
	}

	resp, err := s.client.GetObject(ctx, input)
	if err != nil {
		return object.Object{}, nil, mapError(err)
	}

	return responseToObject(key, resp), resp.Body, nil
}

// Stat returns metadata only.
func (s *Storage) Stat(ctx context.Context, key string) (object.Object, error) {
	if err := s.ensureClient(); err != nil {
		return object.Object{}, err
	}

	resp, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return object.Object{}, mapError(err)
	}

	return headToObject(key, resp), nil
}

// Delete removes an object.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := s.ensureClient(); err != nil {
		return err
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return mapError(err)
}

func (s *Storage) ensureClient() error {
	if s.client == nil {
		return errors.New("r2: client not initialized")
	}
	return nil
}

func responseToObject(key string, resp *s3.GetObjectOutput) object.Object {
	return object.Object{
		Key:          key,
		Size:         aws.ToInt64(resp.ContentLength),
		ETag:         aws.ToString(resp.ETag),
		ContentType:  aws.ToString(resp.ContentType),
		LastModified: aws.ToTime(resp.LastModified),
		CustomMeta:   cloneMeta(resp.Metadata),
	}
}

func headToObject(key string, resp *s3.HeadObjectOutput) object.Object {
	return object.Object{
		Key:          key,
		Size:         aws.ToInt64(resp.ContentLength),
		ETag:         aws.ToString(resp.ETag),
		ContentType:  aws.ToString(resp.ContentType),
		LastModified: aws.ToTime(resp.LastModified),
		CustomMeta:   cloneMeta(resp.Metadata),
	}
}

func cloneMeta(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func rangeHeader(rng object.Range) string {
	if rng.End >= 0 {
		return fmt.Sprintf("bytes=%d-%d", rng.Start, rng.End)
	}
	return fmt.Sprintf("bytes=%d-", rng.Start)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}

	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return object.ErrNotFound
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch strings.ToLower(apiErr.ErrorCode()) {
		case "nosuchkey", "notfound", "404":
			return object.ErrNotFound
		}
	}

	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return object.ErrNotFound
	}

	return err
}
