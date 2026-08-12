package steg

import (
	"bytes"
	"testing"
)

func TestShamirRoundTrip(t *testing.T) {
	secret := []byte("门限分片测试密钥 - Threshold Secret Sharing")
	cases := []struct{ total, need int }{
		{3, 2}, {3, 3}, {5, 3}, {7, 4}, {10, 5}, {1, 1},
	}
	for _, c := range cases {
		shares, err := SplitSecret(secret, c.total, c.need)
		if err != nil {
			t.Fatalf("(%d,%d) SplitSecret: %v", c.total, c.need, err)
		}
		if len(shares) != c.total {
			t.Fatalf("(%d,%d) 分片数量应为 %d, 实际 %d", c.total, c.need, c.total, len(shares))
		}
		// 用恰好 need 个分片恢复
		got, err := RecoverSecret(shares[:c.need], c.need)
		if err != nil {
			t.Fatalf("(%d,%d) RecoverSecret: %v", c.total, c.need, err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("(%d,%d) 恢复结果不一致", c.total, c.need)
		}
	}
}

func TestShamirRecoverAnySubset(t *testing.T) {
	secret := []byte("any-k-of-n")
	shares, err := SplitSecret(secret, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	// 任意 3 个分片都能恢复（选择不同组合）
	combos := [][3]int{{0, 1, 2}, {0, 2, 4}, {1, 3, 4}, {2, 3, 4}}
	for _, c := range combos {
		got, err := RecoverSecret([][]byte{shares[c[0]], shares[c[1]], shares[c[2]]}, 3)
		if err != nil {
			t.Fatalf("组合 %v: %v", c, err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("组合 %v 恢复不一致", c)
		}
	}
}

func TestShamirNotEnoughShares(t *testing.T) {
	secret := []byte("need 3 shares")
	shares, err := SplitSecret(secret, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverSecret(shares[:2], 3); err == nil {
		t.Fatal("分片不足时应恢复失败")
	}
}

func TestShamirInvalidParams(t *testing.T) {
	if _, err := SplitSecret([]byte("x"), 0, 1); err == nil {
		t.Fatal("total=0 应报错")
	}
	if _, err := SplitSecret([]byte("x"), 3, 0); err == nil {
		t.Fatal("need=0 应报错")
	}
	if _, err := SplitSecret([]byte("x"), 3, 4); err == nil {
		t.Fatal("need>total 应报错")
	}
	if _, err := SplitSecret(nil, 3, 2); err == nil {
		t.Fatal("空秘密应报错")
	}
}

func TestShamirWrongShareRejected(t *testing.T) {
	secret := []byte("tamper check")
	shares, err := SplitSecret(secret, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	// 篡改一个分片
	bad := append([]byte(nil), shares[0]...)
	if len(bad) > 0 {
		bad[len(bad)-1] ^= 0xFF
	}
	// 2-of-3 时用 (篡改的, 正常的) —— 可能恢复失败或结果错误，但必须可感知
	got, err := RecoverSecret([][]byte{bad, shares[1]}, 2)
	if err == nil && bytes.Equal(got, secret) {
		t.Fatal("篡改分片后不应恢复出原秘密（或应报错）")
	}
}

func TestShamirLargeSecret(t *testing.T) {
	secret := bytes.Repeat([]byte("0123456789abcdef"), 1024) // 16KB
	shares, err := SplitSecret(secret, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RecoverSecret(shares[:2], 2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("大秘密往返不一致")
	}
}
