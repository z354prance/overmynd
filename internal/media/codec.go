// Package media reads the video codec of a file straight from its container header, so
// Overmynd can tell AV1 from everything else without ffprobe.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
)

const (
	idEBML    = 0x1A45DFA3
	idSegment = 0x18538067
	idTracks  = 0x1654AE6B
	idCluster = 0x1F43B675
	idEntry   = 0xAE
	idType    = 0x83
	idCodec   = 0x86
)

// VideoCodec returns the first video stream's codec in a short lower-case form
// ("av1", "h264", "hevc", "vp9", "mpeg2", ...). Matroska/WebM and MP4/MOV are read.
func VideoCodec(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var head [12]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return "", errors.New("file too short")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	switch {
	case head[0] == 0x1A && head[1] == 0x45 && head[2] == 0xDF && head[3] == 0xA3:
		return mkvCodec(f)
	case string(head[4:8]) == "ftyp" || string(head[4:8]) == "moov" || string(head[4:8]) == "mdat" || string(head[4:8]) == "free":
		return mp4Codec(f)
	}
	return "", errors.New("not a Matroska or MP4 file")
}

// Normalize maps a Matroska CodecID or an MP4 sample entry to the short name.
func Normalize(id string) string {
	l := strings.ToLower(strings.TrimSpace(id))
	switch {
	case l == "v_av1" || l == "av01":
		return "av1"
	case strings.HasPrefix(l, "v_mpeg4/iso/avc") || l == "avc1" || l == "avc3":
		return "h264"
	case strings.HasPrefix(l, "v_mpegh/iso/hevc") || l == "hvc1" || l == "hev1":
		return "hevc"
	case l == "v_vp9" || l == "vp09":
		return "vp9"
	case l == "v_vp8":
		return "vp8"
	case strings.HasPrefix(l, "v_mpeg2") || l == "mp2v":
		return "mpeg2"
	case l == "v_ms/vfw/fourcc" || l == "wvc1":
		return "vc1"
	}
	return l
}

func readID(r io.Reader) (uint64, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	n := 1
	for m := byte(0x80); n <= 4 && b[0]&m == 0; m >>= 1 {
		n++
	}
	if n > 4 {
		return 0, errors.New("bad element id")
	}
	id := uint64(b[0])
	for i := 1; i < n; i++ {
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		id = id<<8 | uint64(b[0])
	}
	return id, nil
}

// readSize returns the data size, or -1 for an unknown size.
func readSize(r io.Reader) (int64, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	n := 1
	m := byte(0x80)
	for n <= 8 && b[0]&m == 0 {
		n++
		m >>= 1
	}
	if n > 8 {
		return 0, errors.New("bad element size")
	}
	v := uint64(b[0] & (m - 1))
	ones := b[0]&(m-1) == m-1
	for i := 1; i < n; i++ {
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		v = v<<8 | uint64(b[0])
		if b[0] != 0xFF {
			ones = false
		}
	}
	if ones {
		return -1, nil
	}
	return int64(v), nil
}

func mkvCodec(f *os.File) (string, error) {
	for {
		id, err := readID(f)
		if err != nil {
			return "", errors.New("no video track found")
		}
		size, err := readSize(f)
		if err != nil {
			return "", err
		}
		switch id {
		case idSegment:
			continue // step inside; its children follow directly
		case idTracks:
			if size < 0 || size > 4<<20 {
				return "", errors.New("unreasonable track list")
			}
			body := make([]byte, size)
			if _, err := io.ReadFull(f, body); err != nil {
				return "", err
			}
			if c := tracksVideo(body); c != "" {
				return Normalize(c), nil
			}
			return "", errors.New("no video track found")
		case idCluster:
			return "", errors.New("no video track found before the data")
		}
		if size < 0 {
			return "", errors.New("unknown-size element")
		}
		if _, err := f.Seek(size, io.SeekCurrent); err != nil {
			return "", err
		}
	}
}

func tracksVideo(b []byte) string {
	r := bytes.NewReader(b)
	for r.Len() > 0 {
		id, err := readID(r)
		if err != nil {
			return ""
		}
		size, err := readSize(r)
		if err != nil || size < 0 || size > int64(r.Len()) {
			return ""
		}
		body := make([]byte, size)
		_, _ = io.ReadFull(r, body)
		if id != idEntry {
			continue
		}
		var typ uint64
		var codec string
		er := bytes.NewReader(body)
		for er.Len() > 0 {
			eid, err := readID(er)
			if err != nil {
				break
			}
			esz, err := readSize(er)
			if err != nil || esz < 0 || esz > int64(er.Len()) {
				break
			}
			v := make([]byte, esz)
			_, _ = io.ReadFull(er, v)
			switch eid {
			case idType:
				typ = 0
				for _, c := range v {
					typ = typ<<8 | uint64(c)
				}
			case idCodec:
				codec = strings.TrimRight(string(v), "\x00")
			}
		}
		if typ == 1 && codec != "" {
			return codec
		}
	}
	return ""
}

func mp4Codec(f *os.File) (string, error) {
	var pos int64
	for {
		var h [8]byte
		if _, err := f.ReadAt(h[:], pos); err != nil {
			return "", errors.New("no video track found")
		}
		size := int64(binary.BigEndian.Uint32(h[:4]))
		typ := string(h[4:8])
		hdr := int64(8)
		if size == 1 {
			var ext [8]byte
			if _, err := f.ReadAt(ext[:], pos+8); err != nil {
				return "", err
			}
			size = int64(binary.BigEndian.Uint64(ext[:]))
			hdr = 16
		} else if size == 0 {
			st, err := f.Stat()
			if err != nil {
				return "", err
			}
			size = st.Size() - pos
		}
		if size < hdr {
			return "", errors.New("corrupt box")
		}
		if typ == "moov" {
			if size > 64<<20 {
				return "", errors.New("unreasonable moov box")
			}
			body := make([]byte, size-hdr)
			if _, err := f.ReadAt(body, pos+hdr); err != nil && err != io.EOF {
				return "", err
			}
			best, at := "", len(body)+1
			for _, c := range []string{"av01", "avc1", "avc3", "hvc1", "hev1", "vp09", "mp2v", "wvc1"} {
				if i := bytes.Index(body, []byte(c)); i >= 0 && i < at {
					best, at = c, i
				}
			}
			if best == "" {
				return "", errors.New("no video track found")
			}
			return Normalize(best), nil
		}
		pos += size
	}
}
