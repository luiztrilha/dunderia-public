package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
)

const (
	ProviderNone = "none"
	ProviderGCS  = "gcs"
)

type Settings struct {
	Provider string
	Bucket   string
	Prefix   string
}

type Sink interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Close() error
}

type sinkFactory func(ctx context.Context, settings Settings) (Sink, error)

var (
	factoryMu      sync.RWMutex
	currentFactory sinkFactory = openSink
)

func NormalizeProvider(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", ProviderNone:
		return ProviderNone
	case ProviderGCS:
		return ProviderGCS
	default:
		return ""
	}
}

func (s Settings) Normalized() Settings {
	s.Provider = NormalizeProvider(s.Provider)
	s.Bucket = strings.TrimSpace(s.Bucket)
	s.Prefix = normalizePrefix(s.Prefix)
	return s
}

func (s Settings) Enabled() bool {
	s = s.Normalized()
	return s.Provider != ProviderNone && s.Provider != "" && s.Bucket != ""
}

func (s Settings) ObjectKey(key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if s.Prefix == "" {
		return key
	}
	return path.Join(s.Prefix, key)
}

func Open(ctx context.Context, settings Settings) (Sink, error) {
	settings = settings.Normalized()
	if !settings.Enabled() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	factoryMu.RLock()
	factory := currentFactory
	factoryMu.RUnlock()
	return factory(ctx, settings)
}

func MirrorBytes(ctx context.Context, settings Settings, key string, data []byte, contentType string) error {
	sink, err := Open(ctx, settings)
	if err != nil || sink == nil {
		return err
	}
	defer sink.Close()
	return sink.Put(ctx, settings.ObjectKey(key), data, contentType)
}

func ReadBytes(ctx context.Context, settings Settings, key string) ([]byte, error) {
	sink, err := Open(ctx, settings)
	if err != nil || sink == nil {
		return nil, err
	}
	defer sink.Close()
	return sink.Get(ctx, settings.ObjectKey(key))
}

func DeleteObject(ctx context.Context, settings Settings, key string) error {
	sink, err := Open(ctx, settings)
	if err != nil || sink == nil {
		return err
	}
	defer sink.Close()
	return sink.Delete(ctx, settings.ObjectKey(key))
}

func IsNotFound(err error) bool {
	return errors.Is(err, storage.ErrObjectNotExist) || errors.Is(err, os.ErrNotExist)
}

func SetSinkFactoryForTest(factory func(ctx context.Context, settings Settings) (Sink, error)) func() {
	factoryMu.Lock()
	prev := currentFactory
	currentFactory = factory
	factoryMu.Unlock()
	return func() {
		factoryMu.Lock()
		currentFactory = prev
		factoryMu.Unlock()
	}
}

func openSink(ctx context.Context, settings Settings) (Sink, error) {
	switch settings.Provider {
	case ProviderGCS:
		return newGCSSink(ctx, settings)
	case ProviderNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported cloud backup provider %q", settings.Provider)
	}
}

func normalizePrefix(prefix string) string {
	prefix = strings.ReplaceAll(strings.TrimSpace(prefix), "\\", "/")
	prefix = strings.Trim(prefix, "/")
	if prefix == "." {
		return ""
	}
	return prefix
}

type gcsSink struct {
	client *storage.Client
	bucket string
}

func newGCSSink(ctx context.Context, settings Settings) (Sink, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("init gcs client: %w", err)
	}
	return &gcsSink{
		client: client,
		bucket: settings.Bucket,
	}, nil
}

func (g *gcsSink) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("cloud backup object key is required")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	writeCtx := ctx
	if writeCtx == nil {
		writeCtx = context.Background()
	}
	if _, ok := writeCtx.Deadline(); !ok {
		var cancel context.CancelFunc
		writeCtx, cancel = context.WithTimeout(writeCtx, 30*time.Second)
		defer cancel()
	}
	writer := g.client.Bucket(g.bucket).Object(key).NewWriter(writeCtx)
	writer.ContentType = contentType
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return fmt.Errorf("upload gcs://%s/%s: %w", g.bucket, key, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize gcs://%s/%s: %w", g.bucket, key, err)
	}
	return nil
}

func (g *gcsSink) Get(ctx context.Context, key string) ([]byte, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("cloud backup object key is required")
	}
	readCtx := ctx
	if readCtx == nil {
		readCtx = context.Background()
	}
	if _, ok := readCtx.Deadline(); !ok {
		var cancel context.CancelFunc
		readCtx, cancel = context.WithTimeout(readCtx, 30*time.Second)
		defer cancel()
	}
	reader, err := g.client.Bucket(g.bucket).Object(key).NewReader(readCtx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (g *gcsSink) Delete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("cloud backup object key is required")
	}
	deleteCtx := ctx
	if deleteCtx == nil {
		deleteCtx = context.Background()
	}
	if _, ok := deleteCtx.Deadline(); !ok {
		var cancel context.CancelFunc
		deleteCtx, cancel = context.WithTimeout(deleteCtx, 30*time.Second)
		defer cancel()
	}
	return g.client.Bucket(g.bucket).Object(key).Delete(deleteCtx)
}

func (g *gcsSink) Close() error {
	if g == nil || g.client == nil {
		return nil
	}
	return g.client.Close()
}
