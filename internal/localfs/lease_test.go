package localfs

import (
	"context"
	"errors"
	"path/filepath"
	"ta/internal/testfs"
	"testing"
	"time"
)

func TestAcquireLeaseSerializesWaitersAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.lock")
	release, err := AcquireLease(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireLease(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled waiter error = %v, want %v", err, context.DeadlineExceeded)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	reopened, err := AcquireLease(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened(); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseDirectoryJunctionUsesCanonicalIdentity(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	testfs.DirectoryJunction(t, root, link)
	direct := filepath.Join(root, "nested", "resource.lock")
	alias := filepath.Join(link, "nested", "resource.lock")
	if got, want := canonicalLeasePath(alias), canonicalLeasePath(direct); got != want {
		t.Fatalf("lease alias=%q, want %q", got, want)
	}
	release, err := AcquireLease(context.Background(), direct)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireLease(ctx, alias); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("alias bypassed active lease: %v", err)
	}
}
