// Package unixcrypt implements the Unix crypt(3) schemes used by the GL.iNet login: MD5-crypt ($1$),
// SHA-256-crypt ($5$) and SHA-512-crypt ($6$). Go's standard library has none of them, so they are
// implemented here from the published algorithms (Poul-Henning Kamp for $1$, Ulrich Drepper's spec for
// $5$/$6$) and checked against glibc's crypt() in the tests.
package unixcrypt

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"hash"
	"strconv"
	"strings"
)

const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Crypt hashes password with a setting such as "$5$salt$" (the trailing part is ignored).
func Crypt(password, setting string) (string, error) {
	switch {
	case strings.HasPrefix(setting, "$1$"):
		return md5Crypt(password, setting), nil
	case strings.HasPrefix(setting, "$5$"):
		return shaCrypt(password, setting, sha256.New, 32), nil
	case strings.HasPrefix(setting, "$6$"):
		return shaCrypt(password, setting, sha512.New, 64), nil
	}
	return "", errors.New("unsupported crypt algorithm")
}

func b64(b2, b1, b0 byte, n int) string {
	w := uint(b2)<<16 | uint(b1)<<8 | uint(b0)
	out := make([]byte, 0, n)
	for ; n > 0; n-- {
		out = append(out, itoa64[w&0x3f])
		w >>= 6
	}
	return string(out)
}

func md5Crypt(pw, setting string) string {
	salt := strings.TrimPrefix(setting, "$1$")
	if i := strings.IndexByte(salt, '$'); i >= 0 {
		salt = salt[:i]
	}
	if len(salt) > 8 {
		salt = salt[:8]
	}
	p, s := []byte(pw), []byte(salt)

	alt := md5.Sum(append(append(append([]byte{}, p...), s...), p...))
	ctx := md5.New()
	ctx.Write(p)
	ctx.Write([]byte("$1$"))
	ctx.Write(s)
	for n := len(p); n > 0; n -= 16 {
		ctx.Write(alt[:min(16, n)])
	}
	for i := len(p); i > 0; i >>= 1 {
		if i&1 != 0 {
			ctx.Write([]byte{0})
		} else if len(p) > 0 {
			ctx.Write(p[:1])
		} else {
			ctx.Write([]byte{0})
		}
	}
	final := ctx.Sum(nil)
	for i := 0; i < 1000; i++ {
		c := md5.New()
		if i&1 != 0 {
			c.Write(p)
		} else {
			c.Write(final)
		}
		if i%3 != 0 {
			c.Write(s)
		}
		if i%7 != 0 {
			c.Write(p)
		}
		if i&1 != 0 {
			c.Write(final)
		} else {
			c.Write(p)
		}
		final = c.Sum(nil)
	}
	f := final
	out := b64(f[0], f[6], f[12], 4) + b64(f[1], f[7], f[13], 4) + b64(f[2], f[8], f[14], 4) +
		b64(f[3], f[9], f[15], 4) + b64(f[4], f[10], f[5], 4) + b64(0, 0, f[11], 2)
	return "$1$" + salt + "$" + out
}

// shaPerm lists, per output group, which digest bytes form the 24-bit word (Drepper's byte order).
var sha256Perm = [][3]int{{0, 10, 20}, {21, 1, 11}, {12, 22, 2}, {3, 13, 23}, {24, 4, 14}, {15, 25, 5}, {6, 16, 26}, {27, 7, 17}, {18, 28, 8}, {9, 19, 29}}
var sha512Perm = [][3]int{{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4}, {47, 5, 26}, {6, 27, 48}, {28, 49, 7}, {50, 8, 29}, {9, 30, 51},
	{31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13}, {56, 14, 35}, {15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19}, {62, 20, 41}}

func shaCrypt(pw, setting string, newHash func() hash.Hash, hlen int) string {
	prefix := setting[:3]
	rest := setting[3:]
	rounds, customRounds := 5000, false
	if strings.HasPrefix(rest, "rounds=") {
		if end := strings.IndexByte(rest, '$'); end > 0 {
			if n, err := strconv.Atoi(rest[7:end]); err == nil {
				rounds, customRounds = min(max(n, 1000), 999999999), true
				rest = rest[end+1:]
			}
		}
	}
	salt := rest
	if i := strings.IndexByte(salt, '$'); i >= 0 {
		salt = salt[:i]
	}
	if len(salt) > 16 {
		salt = salt[:16]
	}
	p, s := []byte(pw), []byte(salt)
	sum := func(parts ...[]byte) []byte {
		h := newHash()
		for _, x := range parts {
			h.Write(x)
		}
		return h.Sum(nil)
	}
	repeat := func(b []byte, n int) []byte { // first n bytes of b repeated
		out := make([]byte, 0, n)
		for len(out) < n {
			out = append(out, b[:min(len(b), n-len(out))]...)
		}
		return out
	}

	B := sum(p, s, p)
	a := newHash()
	a.Write(p)
	a.Write(s)
	n := len(p)
	for ; n > hlen; n -= hlen {
		a.Write(B)
	}
	a.Write(B[:n])
	for i := len(p); i > 0; i >>= 1 {
		if i&1 != 0 {
			a.Write(B)
		} else {
			a.Write(p)
		}
	}
	DA := a.Sum(nil)

	dp := newHash()
	for i := 0; i < len(p); i++ {
		dp.Write(p)
	}
	P := repeat(dp.Sum(nil), len(p))
	ds := newHash()
	for i := 0; i < 16+int(DA[0]); i++ {
		ds.Write(s)
	}
	S := repeat(ds.Sum(nil), len(s))

	for i := 0; i < rounds; i++ {
		c := newHash()
		if i&1 != 0 {
			c.Write(P)
		} else {
			c.Write(DA)
		}
		if i%3 != 0 {
			c.Write(S)
		}
		if i%7 != 0 {
			c.Write(P)
		}
		if i&1 != 0 {
			c.Write(DA)
		} else {
			c.Write(P)
		}
		DA = c.Sum(nil)
	}

	var enc strings.Builder
	if hlen == 32 {
		for _, g := range sha256Perm {
			enc.WriteString(b64(DA[g[0]], DA[g[1]], DA[g[2]], 4))
		}
		enc.WriteString(b64(0, DA[31], DA[30], 3))
	} else {
		for _, g := range sha512Perm {
			enc.WriteString(b64(DA[g[0]], DA[g[1]], DA[g[2]], 4))
		}
		enc.WriteString(b64(0, 0, DA[63], 2))
	}
	out := prefix
	if customRounds {
		out += "rounds=" + strconv.Itoa(rounds) + "$"
	}
	return out + salt + "$" + enc.String()
}
