package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

const presignTTL = 5 * time.Minute

// AllowedContentTypes is the whitelist of MIME types accepted for upload.
var AllowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// PresignResult holds both URLs returned by a successful presign call.
type PresignResult struct {
	// UploadURL is the short-lived PUT URL the client sends bytes to.
	UploadURL string `json:"upload_url"`
	// PublicURL is the permanent URL to store in the database.
	PublicURL string `json:"public_url"`
}

// Presigner generates short-lived S3-compatible presign upload URLs.
type Presigner interface {
	// Presign validates content_type, generates a unique object key, and
	// returns a PUT URL (expires in 5 min) plus the permanent public URL.
	Presign(ctx context.Context, contentType string) (PresignResult, error)
}

// R2Config holds the Cloudflare R2 credentials.
type R2Config struct {
	AccountID     string
	AccessKeyID   string
	SecretKey     string
	BucketName    string
	PublicBaseURL string // e.g. "https://pub-xxx.r2.dev" — no trailing slash
}

// IsConfigured returns true when all required fields are present.
func (c R2Config) IsConfigured() bool {
	return c.AccountID != "" &&
		c.AccessKeyID != "" &&
		c.SecretKey != "" &&
		c.BucketName != "" &&
		c.PublicBaseURL != ""
}

type r2Presigner struct {
	client    *s3.PresignClient
	bucket    string
	publicURL string
}

// NewR2Presigner builds an R2-backed Presigner using the S3-compatible API.
func NewR2Presigner(cfg R2Config) Presigner {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)

	s3Client := s3.NewFromConfig(aws.Config{
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, ""),
		BaseEndpoint: aws.String(endpoint),
	})

	return &r2Presigner{
		client:    s3.NewPresignClient(s3Client),
		bucket:    cfg.BucketName,
		publicURL: cfg.PublicBaseURL,
	}
}

func (p *r2Presigner) Presign(ctx context.Context, contentType string) (PresignResult, error) {
	if !AllowedContentTypes[contentType] {
		return PresignResult{}, fmt.Errorf("unsupported content_type: %s", contentType)
	}

	key := generateKey(contentType)

	out, err := p.client.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(p.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(presignTTL))
	if err != nil {
		return PresignResult{}, fmt.Errorf("r2 presign: %w", err)
	}

	return PresignResult{
		UploadURL: out.URL,
		PublicURL: p.publicURL + "/" + key,
	}, nil
}

// generateKey builds a collision-free object key.
// Pattern: uploads/<uuid>.<ext>
func generateKey(contentType string) string {
	return "uploads/" + uuid.New().String() + extensionFor(contentType)
}

func extensionFor(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}
