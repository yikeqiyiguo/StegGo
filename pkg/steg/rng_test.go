package steg

import "testing"

func TestRNGDeterministic(t *testing.T) {
	seed := []byte("test-seed")
	r1 := NewRNG(seed)
	r2 := NewRNG(seed)
	for i := 0; i < 1000; i++ {
		if r1.Next() != r2.Next() {
			t.Fatalf("相同种子第 %d 次输出不一致", i)
		}
	}
}

func TestRNGDifferentSeeds(t *testing.T) {
	a := NewRNG([]byte("seed-a"))
	b := NewRNG([]byte("seed-b"))
	same := true
	for i := 0; i < 64; i++ {
		if a.Next() != b.Next() {
			same = false
			break
		}
	}
	if same {
		t.Fatal("不同种子输出不应完全相同")
	}
}

func TestSeedFromPasswordDeterministic(t *testing.T) {
	s1 := SeedFromPassword([]byte("password"))
	s2 := SeedFromPassword([]byte("password"))
	if len(s1) == 0 || len(s2) == 0 {
		t.Fatal("种子不应为空")
	}
	if string(s1) != string(s2) {
		t.Fatal("相同密码应派生相同种子")
	}
	s3 := SeedFromPassword([]byte("password-x"))
	if string(s1) == string(s3) {
		t.Fatal("不同密码不应派生相同种子")
	}
}

func TestRNGNextIntBounds(t *testing.T) {
	r := NewRNG([]byte("bounds"))
	for i := 0; i < 10000; i++ {
		v := r.NextInt(100)
		if v >= 100 {
			t.Fatalf("NextInt 越界: %d", v)
		}
	}
}

func TestPixelCursorCoverage(t *testing.T) {
	seed := []byte("cursor")
	w, h := 64, 48
	c := NewPixelCursor(seed, w, h)
	seen := make(map[int]bool)
	count := 0
	for {
		idx, x, y := c.Next()
		if x < 0 {
			break
		}
		if idx < 0 || idx >= w*h {
			t.Fatalf("idx 越界: %d", idx)
		}
		if x < 0 || x >= w || y < 0 || y >= h {
			t.Fatalf("坐标越界: (%d,%d)", x, y)
		}
		seen[idx] = true
		count++
	}
	if count != w*h {
		t.Fatalf("像素游标应覆盖全部像素: 期望 %d, 实际 %d", w*h, count)
	}
	if len(seen) != w*h {
		t.Fatal("像素游标应无重复覆盖")
	}
}

func TestPixelCursorDeterministic(t *testing.T) {
	seed := []byte("cursor-det")
	c1 := NewPixelCursor(seed, 32, 32)
	c2 := NewPixelCursor(seed, 32, 32)
	for i := 0; i < 100; i++ {
		ai, ax, ay := c1.Next()
		bi, bx, by := c2.Next()
		if ai != bi || ax != bx || ay != by {
			t.Fatalf("第 %d 次坐标不一致: (%d,%d,%d) != (%d,%d,%d)", i, ai, ax, ay, bi, bx, by)
		}
	}
}
