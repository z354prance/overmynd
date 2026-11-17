// Package bencode extracts what intake needs from a .torrent file: the infohash and name.
package bencode

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var errBad = errors.New("invalid torrent data")

// skip returns the index just after the bencoded value starting at i.
func skip(b []byte, i int) (int, error) {
	if i >= len(b) {
		return 0, errBad
	}
	switch c := b[i]; {
	case c == 'i':
		j := i + 1
		for j < len(b) && b[j] != 'e' {
			j++
		}
		if j >= len(b) {
			return 0, errBad
		}
		return j + 1, nil
	case c == 'l' || c == 'd':
		j := i + 1
		for j < len(b) && b[j] != 'e' {
			var err error
			if j, err = skip(b, j); err != nil {
				return 0, err
			}
		}
		if j >= len(b) {
			return 0, errBad
		}
		return j + 1, nil
	case c >= '0' && c <= '9':
		j := i
		for j < len(b) && b[j] != ':' {
			j++
		}
		if j >= len(b) {
			return 0, errBad
		}
		n, err := strconv.Atoi(string(b[i:j]))
		if err != nil || n < 0 || j+1+n > len(b) {
			return 0, errBad
		}
		return j + 1 + n, nil
	}
	return 0, errBad
}

func readString(b []byte, i int) (string, int, error) {
	j := i
	for j < len(b) && b[j] != ':' {
		j++
	}
	if j >= len(b) {
		return "", 0, errBad
	}
	n, err := strconv.Atoi(string(b[i:j]))
	if err != nil || n < 0 || j+1+n > len(b) {
		return "", 0, errBad
	}
	return string(b[j+1 : j+1+n]), j + 1 + n, nil
}

// dictValue returns the raw span of the value for key in the dict starting at i.
func dictValue(b []byte, i int, key string) (start, end int, err error) {
	if i >= len(b) || b[i] != 'd' {
		return 0, 0, errBad
	}
	j := i + 1
	for j < len(b) && b[j] != 'e' {
		k, next, err := readString(b, j)
		if err != nil {
			return 0, 0, err
		}
		vend, err := skip(b, next)
		if err != nil {
			return 0, 0, err
		}
		if k == key {
			return next, vend, nil
		}
		j = vend
	}
	return 0, 0, errors.New("key not found: " + key)
}

// Torrent returns the lowercase hex infohash, the torrent name, and total size.
func Torrent(b []byte) (hash, name string, size int64, err error) {
	s, e, err := dictValue(b, 0, "info")
	if err != nil {
		return "", "", 0, err
	}
	sum := sha1.Sum(b[s:e])
	hash = hex.EncodeToString(sum[:])
	if ns, ne, err := dictValue(b, s, "name"); err == nil {
		name, _, _ = readString(b, ns)
		_ = ne
	}
	if ls, _, err := dictValue(b, s, "length"); err == nil && b[ls] == 'i' {
		j := ls + 1
		for j < len(b) && b[j] != 'e' {
			j++
		}
		size, _ = strconv.ParseInt(string(b[ls+1:j]), 10, 64)
	}
	return hash, name, size, nil
}

// Magnet returns the lowercase hex infohash and display name from a magnet URI.
func Magnet(uri string) (hash, name string, err error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "magnet" {
		return "", "", errors.New("not a magnet link")
	}
	q := u.Query()
	name = q.Get("dn")
	for _, xt := range q["xt"] {
		const p = "urn:btih:"
		if !strings.HasPrefix(strings.ToLower(xt), p) {
			continue
		}
		h := xt[len(p):]
		switch len(h) {
		case 40:
			if _, err := hex.DecodeString(h); err == nil {
				return strings.ToLower(h), name, nil
			}
		case 32:
			raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(h))
			if err == nil {
				return hex.EncodeToString(raw), name, nil
			}
		}
	}
	return "", "", errors.New("magnet has no btih hash")
}
