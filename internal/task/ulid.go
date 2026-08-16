package task

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// crockfordAlphabet はULIDが使うCrockford Base32。I/L/O/Uを除き誤読を避ける。
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ulidLen はULIDの文字数。48bitのtimestamp(10文字)と80bitのrandom(16文字)。
const ulidLen = 26

// NewULID は現在時刻を先頭に持つULIDを生成する。
//
// 外部ライブラリを持ち込まないため自前実装とする。時刻順にソート可能で、
// 人間が目で追えることを重視してUUIDではなくULIDを採用している。
func NewULID() (string, error) {
	return newULIDAt(time.Now())
}

func newULIDAt(t time.Time) (string, error) {
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("read entropy: %w", err)
	}

	var b strings.Builder
	b.Grow(ulidLen)

	// timestampはミリ秒48bitを上位から5bitずつ10文字へ。
	ms := uint64(t.UnixMilli())
	for i := 9; i >= 0; i-- {
		b.WriteByte(crockfordAlphabet[(ms>>(uint(i)*5))&0x1f])
	}

	// randomは80bitを5bitずつ16文字へ。バイト列を跨いでbitを取り出す。
	var bits uint16
	var nbits uint
	for _, by := range entropy {
		bits = bits<<8 | uint16(by)
		nbits += 8
		for nbits >= 5 {
			nbits -= 5
			b.WriteByte(crockfordAlphabet[(bits>>nbits)&0x1f])
		}
	}

	return b.String(), nil
}

// shortIDLen はUI表示に使う短縮IDの長さ。
//
// ULIDの先頭10文字はミリ秒timestampであり、続けて作ったTaskでは同じ値になる。
// 8文字ではtimestampの一部しか映らず、同時期に作ったTaskの短縮IDが揃ってしまい
// 一覧で見分けられない。timestamp全体にrandomの先頭2文字を足して識別性を確保する。
const shortIDLen = 12

// ShortID はUI表示用に短縮したIDを返す。内部では常に完全なIDを使う。
func ShortID(id string) string {
	if len(id) <= shortIDLen {
		return id
	}
	return id[:shortIDLen]
}

// ValidULID はULIDとして妥当な文字列かを返す。
func ValidULID(id string) bool {
	if len(id) != ulidLen {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune(crockfordAlphabet, r) {
			return false
		}
	}
	return true
}
