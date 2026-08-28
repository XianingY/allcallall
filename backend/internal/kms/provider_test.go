package kms

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func randKeyB64() string {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestStaticProviderLoads32ByteKey(t *testing.T) {
	t.Setenv("MESSAGE_ENCRYPTION_MASTER_KEY", randKeyB64())
	p := StaticProvider{}
	key, err := p.GetMasterKey(context.Background(), "local-v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
}

func TestStaticProviderRejectsMissingKey(t *testing.T) {
	t.Setenv("MESSAGE_ENCRYPTION_MASTER_KEY", "")
	if _, err := (StaticProvider{}).GetMasterKey(context.Background(), ""); err == nil {
		t.Fatal("expected error when master key unset")
	}
}

func TestRotatingProviderCachesAndRefetches(t *testing.T) {
	calls := 0
	fetcher := func(_ context.Context, _ string) ([]byte, error) {
		calls++
		k := make([]byte, 32)
		for i := range k {
			k[i] = byte(calls)
		}
		return k, nil
	}
	p := &RotatingProvider{Fetcher: fetcher, TTL: 10 * time.Millisecond}
	ctx := context.Background()

	k1, _ := p.GetMasterKey(ctx, "k1")
	k2, _ := p.GetMasterKey(ctx, "k1")
	if calls != 1 {
		t.Fatalf("expected 1 fetch due to cache, got %d", calls)
	}
	if string(k1) != string(k2) {
		t.Fatal("cached key changed within TTL")
	}
	time.Sleep(15 * time.Millisecond)
	k3, _ := p.GetMasterKey(ctx, "k1")
	if calls != 2 {
		t.Fatalf("expected refetch after TTL, got %d calls", calls)
	}
	if string(k3) == string(k1) {
		t.Fatal("expected new key after TTL expiry")
	}
}

func TestNewCipherRoundtrip(t *testing.T) {
	t.Setenv("MESSAGE_ENCRYPTION_MASTER_KEY", randKeyB64())
	cipher, err := NewCipher(context.Background(), StaticProvider{}, "local-v1")
	if err != nil {
		t.Fatal(err)
	}
	ct, meta, err := cipher.Encrypt("secret message")
	if err != nil || ct == "" || meta == "" {
		t.Fatalf("encrypt failed: ct=%q meta=%q err=%v", ct, meta, err)
	}
	pt, err := cipher.Decrypt(ct, meta)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "secret message" {
		t.Fatalf("roundtrip mismatch: %q", pt)
	}
}
