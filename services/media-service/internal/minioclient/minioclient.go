package minioclient

import (
	"context"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	raw        *minio.Client
	bucket     string
	publicBase *url.URL
}

func New(internalEndpoint, accessKeyID, secretAccessKey, bucket, publicBaseURL string) (*Client, error) {
	raw, err := minio.New(internalEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}

	base, err := url.Parse(publicBaseURL)
	if err != nil {
		return nil, err
	}

	return &Client{raw: raw, bucket: bucket, publicBase: base}, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.raw.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return c.raw.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{})
}

func (c *Client) PresignedPutURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	u, err := c.raw.PresignedPutObject(ctx, c.bucket, objectKey, expires)
	if err != nil {
		return "", err
	}
	return c.rewriteToPublic(u).String(), nil
}

func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	u, err := c.raw.PresignedGetObject(ctx, c.bucket, objectKey, expires, nil)
	if err != nil {
		return "", err
	}
	return c.rewriteToPublic(u).String(), nil
}

func (c *Client) StatObjectExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := c.raw.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if resp := minio.ToErrorResponse(err); resp.Code == minio.NoSuchKey {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Client) rewriteToPublic(u *url.URL) *url.URL {
	out := *u
	out.Scheme = c.publicBase.Scheme
	out.Host = c.publicBase.Host
	out.Path = c.publicBase.Path + trimBucketPrefix(u.Path, c.bucket)
	return &out
}

func trimBucketPrefix(path, bucket string) string {
	prefix := "/" + bucket
	if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
		return path[len(prefix):]
	}
	return path
}
