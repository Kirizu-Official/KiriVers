package grayutil

import (
	"encoding/hex"
	"testing"
)

const testSalt = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestBucketDeterministic 同一 (salt, device) 多次计算必须得到同一桶值（灰度判定恒定）。
func TestBucketDeterministic(t *testing.T) {
	first := Bucket(testSalt, "device-abc")
	for i := 0; i < 100; i++ {
		if got := Bucket(testSalt, "device-abc"); got != first {
			t.Fatalf("bucket not deterministic: %d != %d", got, first)
		}
	}
	// 不同设备桶值应不同（碰撞概率 1/10000，固定样例下直接断言不同）
	if Bucket(testSalt, "device-abc") == Bucket(testSalt, "device-xyz") {
		t.Fatalf("expected different buckets for different devices")
	}
	// 不同 salt 下结果不同（版本间独立）
	if Bucket(testSalt, "device-abc") == Bucket("ff"+testSalt[2:], "device-abc") {
		t.Fatalf("expected different buckets for different salts")
	}
}

// TestBucketRange 桶值必须落在 [0, 10000)。
func TestBucketRange(t *testing.T) {
	for i := 0; i < 2000; i++ {
		id := "device-" + hex.EncodeToString([]byte{byte(i >> 8), byte(i)})
		if b := Bucket(testSalt, id); b >= 10000 {
			t.Fatalf("bucket %d out of range for %s", b, id)
		}
	}
}

// TestHitBoundaries 灰度边界：0/100/匿名与负数百分比。
func TestHitBoundaries(t *testing.T) {
	if Hit(testSalt, "device-abc", 100) != true {
		t.Fatalf("percent=100 must always hit")
	}
	if Hit(testSalt, "device-abc", 0) != false {
		t.Fatalf("percent=0 must never hit")
	}
	if Hit(testSalt, "device-abc", -5) != false {
		t.Fatalf("negative percent must never hit")
	}
	// 匿名（无 device_id）在非 100% 下未命中（§5.4）
	for p := 1; p <= 99; p++ {
		if Hit(testSalt, "", p) {
			t.Fatalf("anonymous device must not hit percent=%d", p)
		}
	}
}

// TestHitDistribution 大样本下命中率应接近配置百分比（±5%）。
func TestHitDistribution(t *testing.T) {
	for _, percent := range []int{10, 50, 90} {
		hit := 0
		const total = 10000
		for i := 0; i < total; i++ {
			id := "device-" + hex.EncodeToString([]byte{byte(i >> 8), byte(i & 0xff)})
			if Hit(testSalt, id, percent) {
				hit++
			}
		}
		want := total * percent / 100
		if diff := hit - want; diff < -500 || diff > 500 {
			t.Fatalf("percent=%d hit=%d want~%d", percent, hit, want)
		}
	}
}

// TestInvalidSalt 非法十六进制 salt 不得 panic，且判定确定。
func TestInvalidSalt(t *testing.T) {
	a := Bucket("not-hex-salt", "device-1")
	b := Bucket("not-hex-salt", "device-1")
	if a != b {
		t.Fatalf("invalid salt must stay deterministic")
	}
	if Bucket("not-hex-salt", "device-1") == Bucket("another-salt", "device-1") {
		t.Fatalf("different invalid salts should differ")
	}
}
