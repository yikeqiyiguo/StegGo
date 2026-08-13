package ecc

import (
	"fmt"
	"testing"
)

func TestDebugBM(t *testing.T) {
	nsym := 4
	dataLen := 255 - nsym
	data := make([]byte, dataLen)
	for i := range data {
		data[i] = byte(i)
	}
	g := rsGeneratorPoly(nsym)
	code := rsEncode(data, g, nsym)
	bad := append([]byte(nil), code...)
	bad[3] ^= 0x5A
	synd := rsCalcSyndromes(bad, nsym)
	fmt.Printf("synd=%v\n", synd)

	// 逐步 BM
	errLoc := []byte{1}
	oldLoc := []byte{1}
	for i := 0; i < nsym; i++ {
		delta := synd[i]
		for j := 1; j < len(errLoc); j++ {
			delta ^= gfMul(errLoc[len(errLoc)-j-1], synd[i-j])
		}
		fmt.Printf("i=%d delta=%d errLoc=%v\n", i, delta, errLoc)
		oldLoc = append(oldLoc, 0)
		if delta != 0 {
			if len(oldLoc) > len(errLoc) {
				newLoc := gfPolyScale(oldLoc, delta)
				oldLoc = gfPolyScale(errLoc, gfInverse(delta))
				errLoc = newLoc
			}
			errLoc = gfPolyAdd(errLoc, gfPolyScale(oldLoc, delta))
		}
		fmt.Printf("  -> errLoc=%v oldLoc=%v\n", errLoc, oldLoc)
	}
	errCount := len(errLoc) - 1
	fmt.Printf("errCount=%d (应为1)\n", errCount)
}

func TestDebugSingleBlock(t *testing.T) {
	nsym := 4
	dataLen := 255 - nsym
	data := make([]byte, dataLen)
	for i := range data {
		data[i] = byte(i)
	}
	g := rsGeneratorPoly(nsym)
	code := rsEncode(data, g, nsym)
	bad := append([]byte(nil), code...)
	bad[3] ^= 0x5A
	synd := rsCalcSyndromes(bad, nsym)
	fmt.Printf("synd=%v (synd[0]=%d 应为 0x5A=90)\n", synd, synd[0])

	errLoc, err := rsFindErrorLocator(synd, nsym)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("errLoc=%v\n", errLoc)

	positions, err := rsFindErrors(errLoc, len(bad))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("positions=%v (应为 [3])\n", positions)

	fixed, err := rsCorrectErrors(bad, synd, positions)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("fixed==code? %v\n", string(fixed) == string(code))
	// 复算伴随式确认已修复
	synd2 := rsCalcSyndromes(fixed, nsym)
	fmt.Printf("synd after fix=%v\n", synd2)

	// 额外：直接检查 Forney 内部值
	if len(positions) == 1 {
		p := positions[0]
		cp := len(bad) - 1 - p
		X := gfPow(2, cp)
		XInv := gfInverse(X)
		fmt.Printf("coefPos=%d X=%d XInv=%d (期望 alpha^251)\n", cp, X, XInv)
		syndRev := append([]byte(nil), synd...)
		reverseBytes(syndRev)
		prod := gfPolyMul(syndRev, []byte{X, 1})
		var rem []byte
		if len(prod) <= 1 {
			rem = append([]byte(nil), prod...)
		} else {
			rem = append([]byte(nil), prod[len(prod)-1:]...)
		}
		reverseBytes(rem)
		fmt.Printf("omega(rem)=%v, omegaVal=%d\n", rem, gfPolyEvalLowFirst(rem, XInv))
		fmt.Printf("X^(1-1)=%d, errLocPrime=%d\n", gfPow(X, 0), 1)
		fmt.Printf("magnitude=%d (应为 90)\n", gfMul(gfPow(X, 0), gfPolyEvalLowFirst(rem, XInv)))
	}
}

func TestDebugTwoErrors(t *testing.T) {
	nsym := 8 // 8 冗余，可纠 4 错
	dataLen := 255 - nsym
	data := make([]byte, dataLen)
	for i := range data {
		data[i] = byte(i)
	}
	g := rsGeneratorPoly(nsym)
	code := rsEncode(data, g, nsym)
	bad := append([]byte(nil), code...)
	// 注入 2 个错误
	bad[3] ^= 0x5A
	bad[200] ^= 0xA5
	synd := rsCalcSyndromes(bad, nsym)
	fmt.Printf("synd=%v\n", synd)

	errLoc, err := rsFindErrorLocator(synd, nsym)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("errLoc=%v\n", errLoc)

	positions, err := rsFindErrors(errLoc, len(bad))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("positions=%v (应为 [3 200])\n", positions)

	fixed, err := rsCorrectErrors(bad, synd, positions)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("fixed==code? %v\n", string(fixed) == string(code))
	synd2 := rsCalcSyndromes(fixed, nsym)
	fmt.Printf("synd after fix=%v (应全 0)\n", synd2)
	if string(fixed) != string(code) {
		t.Fatal("two-error correction failed")
	}
}
