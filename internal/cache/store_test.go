package cache

import (
	"bytes"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
)

func TestOpenUnknownDriver(t *testing.T) {
	_, err := Open(Options{Driver: "memcached"})
	if err == nil {
		t.Fatal("expected unknown driver error")
	}
}

func TestOpenRedisPingFail(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()
	_, err := Open(Options{
		Driver:      "redis",
		RedisAddr:   addr,
		PingTimeout: 200 * time.Millisecond,
		DialTimeout: 200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected ping failure")
	}
}

func TestMemoryStoreContract(t *testing.T) {
	store, err := Open(Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertStoreContract(t, store)
}

func TestRedisStoreContract(t *testing.T) {
	mr := miniredis.RunT(t)
	store, err := Open(Options{
		Driver:            "redis",
		RedisAddr:         mr.Addr(),
		ReconnectInterval: time.Hour,
		PingTimeout:       time.Second,
		DialTimeout:       time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	assertStoreContract(t, store)
}

func assertStoreContract(t *testing.T, store Store) {
	t.Helper()
	ctx := t.Context()
	got, ok, err := store.Get(ctx, "kirivers:missing")
	if err != nil || ok || got != nil {
		t.Fatalf("miss: val=%v ok=%v err=%v", got, ok, err)
	}
	if err := store.Set(ctx, "kirivers:a", []byte("one")); err != nil {
		t.Fatal(err)
	}
	got, ok, err = store.Get(ctx, "kirivers:a")
	if err != nil || !ok || !bytes.Equal(got, []byte("one")) {
		t.Fatalf("get: val=%q ok=%v err=%v", got, ok, err)
	}
	got[0] = 'x'
	got, ok, err = store.Get(ctx, "kirivers:a")
	if err != nil || !ok || !bytes.Equal(got, []byte("one")) {
		t.Fatal("Get must clone bytes")
	}
	if err := store.Set(ctx, "kirivers:catalog:p:linux:x86_64", []byte("cat")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, "kirivers:project:id:p", []byte("proj")); err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePrefix(ctx, "kirivers:catalog:p:"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = store.Get(ctx, "kirivers:catalog:p:linux:x86_64")
	if err != nil || ok {
		t.Fatal("catalog prefix should be gone")
	}
	got, ok, err = store.Get(ctx, "kirivers:project:id:p")
	if err != nil || !ok || !bytes.Equal(got, []byte("proj")) {
		t.Fatal("unrelated key must remain")
	}
	if err := store.Delete(ctx, "kirivers:a", "kirivers:project:id:p"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = store.Get(ctx, "kirivers:a")
	if err != nil || ok {
		t.Fatal("deleted key still present")
	}
	ok, err = store.SetNX(ctx, "kirivers:nx", []byte("first"), time.Minute)
	if err != nil || !ok {
		t.Fatalf("setnx first: ok=%v err=%v", ok, err)
	}
	ok, err = store.SetNX(ctx, "kirivers:nx", []byte("second"), time.Minute)
	if err != nil || ok {
		t.Fatalf("setnx second must lose: ok=%v err=%v", ok, err)
	}
	got, hit, err := store.Get(ctx, "kirivers:nx")
	if err != nil || !hit || !bytes.Equal(got, []byte("first")) {
		t.Fatalf("setnx value=%q hit=%v", got, hit)
	}
}

func TestInvalidateProjectDeletesLookupsAndCatalog(t *testing.T) {
	store, err := Open(Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	slugKey := ProjectSlugKey("app")
	if err := store.Set(ctx, ProjectIDKey(id), []byte("id")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, slugKey, []byte("slug")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, ProjectLookupsKey(id), []byte(`["`+slugKey+`"]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, CatalogKey(id, "linux", "x86_64"), []byte("cat")); err != nil {
		t.Fatal(err)
	}
	if err := InvalidateProject(ctx, store, id); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{ProjectIDKey(id), slugKey, ProjectLookupsKey(id), CatalogKey(id, "linux", "x86_64")} {
		_, ok, err := store.Get(ctx, key)
		if err != nil || ok {
			t.Fatalf("key %s still present", key)
		}
	}
}

func TestFailoverFallsBackThenReconnectsAndFlushes(t *testing.T) {
	mr := miniredis.RunT(t)
	store, err := Open(Options{
		Driver:            "redis",
		RedisAddr:         mr.Addr(),
		ReconnectInterval: 40 * time.Millisecond,
		PingTimeout:       200 * time.Millisecond,
		DialTimeout:       200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.Set(ctx, "kirivers:stale", []byte("old")); err != nil {
		t.Fatal(err)
	}
	mr.Close()

	if err := store.Set(ctx, "kirivers:mem", []byte("standby")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, "kirivers:mem")
	if err != nil || !ok || string(got) != "standby" {
		t.Fatalf("memory fallback get: val=%q ok=%v err=%v", got, ok, err)
	}

	if err := mr.Restart(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		_, memOK, err := store.Get(ctx, "kirivers:mem")
		if err != nil {
			t.Fatal(err)
		}
		if !mr.Exists("kirivers:stale") && !memOK {
			if err := store.Set(ctx, "kirivers:fresh", []byte("ok")); err != nil {
				t.Fatal(err)
			}
			if mr.Exists("kirivers:fresh") {
				recovered = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !recovered {
		t.Fatal("expected reconnect to flush kirivers: prefix, drop memory, and write to redis")
	}
	_, ok, err = store.Get(ctx, "kirivers:stale")
	if err != nil || ok {
		t.Fatalf("stale redis key should be gone after reconnect, ok=%v err=%v", ok, err)
	}
}

func TestOpenMemoryDoesNotNeedRedis(t *testing.T) {
	store, err := Open(Options{Driver: "memory", RedisAddr: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
}

func TestMemorySetWithTTLExpiresAsMiss(t *testing.T) {
	store, err := Open(Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.SetWithTTL(ctx, "kirivers:admin:sess:t", []byte(`{"admin_id":"x"}`), 40*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, "kirivers:admin:sess:t")
	if err != nil || !ok || string(got) != `{"admin_id":"x"}` {
		t.Fatalf("before expiry: val=%q ok=%v err=%v", got, ok, err)
	}
	time.Sleep(60 * time.Millisecond)
	got, ok, err = store.Get(ctx, "kirivers:admin:sess:t")
	if err != nil || ok || got != nil {
		t.Fatalf("expired must be miss: val=%q ok=%v err=%v", got, ok, err)
	}
}

func TestSetWithTTLRejectsNonPositive(t *testing.T) {
	store, err := Open(Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.SetWithTTL(ctx, "k", []byte("v"), 0); err == nil {
		t.Fatal("ttl=0 must fail")
	}
	if err := store.SetWithTTL(ctx, "k", []byte("v"), -time.Second); err == nil {
		t.Fatal("negative ttl must fail")
	}
}

func TestRedisSetWithTTLExpires(t *testing.T) {
	mr := miniredis.RunT(t)
	store, err := Open(Options{
		Driver:            "redis",
		RedisAddr:         mr.Addr(),
		ReconnectInterval: time.Hour,
		PingTimeout:       time.Second,
		DialTimeout:       time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.SetWithTTL(ctx, "kirivers:admin:pend:t", []byte("pend"), time.Second); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, "kirivers:admin:pend:t")
	if err != nil || !ok || string(got) != "pend" {
		t.Fatalf("redis ttl get: val=%q ok=%v err=%v", got, ok, err)
	}
	mr.FastForward(2 * time.Second)
	got, ok, err = store.Get(ctx, "kirivers:admin:pend:t")
	if err != nil || ok || got != nil {
		t.Fatalf("redis expired must be miss: val=%q ok=%v err=%v", got, ok, err)
	}
}

func TestImmortalSetSurvivesTTLPath(t *testing.T) {
	store, err := Open(Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	if err := store.Set(ctx, "kirivers:catalog:p:linux:x86_64", []byte("cat")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	got, ok, err := store.Get(ctx, "kirivers:catalog:p:linux:x86_64")
	if err != nil || !ok || string(got) != "cat" {
		t.Fatal("immortal Set must not expire")
	}
}
