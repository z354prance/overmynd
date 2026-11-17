package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const (
	idVideo = 0xE0
	idPixW  = 0xB0
	idPixH  = 0xBA
)

// VideoSize returns the pixel width and height of the first video stream, read from the
// container header (Matroska/WebM and MP4/MOV).
func VideoSize(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var head [12]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return 0, 0, errors.New("file too short")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, err
	}
	switch {
	case head[0] == 0x1A && head[1] == 0x45 && head[2] == 0xDF && head[3] == 0xA3:
		return mkvSize(f)
	case string(head[4:8]) == "ftyp" || string(head[4:8]) == "moov" || string(head[4:8]) == "mdat" || string(head[4:8]) == "free":
		return mp4Size(f)
	}
	return 0, 0, errors.New("not a Matroska or MP4 file")
}

// Tier names the usual resolution step a frame size belongs to: 2160, 1080, 720, 480 or
// 360 (anything smaller). Letterboxed widescreen frames count by their width.
func Tier(w, h int) int {
	switch {
	case w >= 3000 || h >= 2000:
		return 2160
	case w >= 1800 || h >= 1000:
		return 1080
	case w >= 1200 || h >= 700:
		return 720
	case w >= 600 || h >= 450:
		return 480
	}
	return 360
}

// ResolutionTier maps an arr's resolution number (360, 480, 540, 576, 720, 1080, 2160)
// onto the same steps.
func ResolutionTier(r int) int {
	switch {
	case r >= 2160:
		return 2160
	case r >= 1080:
		return 1080
	case r >= 720:
		return 720
	case r >= 480:
		return 480
	case r > 0:
		return 360
	}
	return 0
}

func elements(b []byte, fn func(id uint64, v []byte)) {
	r := bytes.NewReader(b)
	for r.Len() > 0 {
		id, err := readID(r)
		if err != nil {
			return
		}
		size, err := readSize(r)
		if err != nil || size < 0 || size > int64(r.Len()) {
			return
		}
		v := make([]byte, size)
		_, _ = io.ReadFull(r, v)
		fn(id, v)
	}
}

func beInt(b []byte) (n int) {
	for _, c := range b {
		n = n<<8 | int(c)
	}
	return n
}

func mkvSize(f *os.File) (int, int, error) {
	for {
		id, err := readID(f)
		if err != nil {
			return 0, 0, errors.New("no video track found")
		}
		size, err := readSize(f)
		if err != nil {
			return 0, 0, err
		}
		switch id {
		case idSegment:
			continue
		case idTracks:
			if size < 0 || size > 4<<20 {
				return 0, 0, errors.New("unreasonable track list")
			}
			body := make([]byte, size)
			if _, err := io.ReadFull(f, body); err != nil {
				return 0, 0, err
			}
			var w, h int
			elements(body, func(id uint64, v []byte) {
				if id != idEntry || w != 0 {
					return
				}
				var typ, tw, th int
				elements(v, func(id uint64, v []byte) {
					switch id {
					case idType:
						typ = beInt(v)
					case idVideo:
						elements(v, func(id uint64, v []byte) {
							switch id {
							case idPixW:
								tw = beInt(v)
							case idPixH:
								th = beInt(v)
							}
						})
					}
				})
				if typ == 1 && tw > 0 && th > 0 {
					w, h = tw, th
				}
			})
			if w == 0 {
				return 0, 0, errors.New("no video size found")
			}
			return w, h, nil
		case idCluster:
			return 0, 0, errors.New("no video track found before the data")
		}
		if size < 0 {
			return 0, 0, errors.New("unknown-size element")
		}
		if _, err := f.Seek(size, io.SeekCurrent); err != nil {
			return 0, 0, err
		}
	}
}

func mp4Size(f *os.File) (int, int, error) {
	var pos int64
	for {
		var h [8]byte
		if _, err := f.ReadAt(h[:], pos); err != nil {
			return 0, 0, errors.New("no video track found")
		}
		size := int64(binary.BigEndian.Uint32(h[:4]))
		hdr := int64(8)
		if size == 1 {
			var ext [8]byte
			if _, err := f.ReadAt(ext[:], pos+8); err != nil {
				return 0, 0, err
			}
			size = int64(binary.BigEndian.Uint64(ext[:]))
			hdr = 16
		} else if size == 0 {
			st, err := f.Stat()
			if err != nil {
				return 0, 0, err
			}
			size = st.Size() - pos
		}
		if size < hdr {
			return 0, 0, errors.New("corrupt box")
		}
		if string(h[4:8]) == "moov" {
			if size > 64<<20 {
				return 0, 0, errors.New("unreasonable moov box")
			}
			body := make([]byte, size-hdr)
			if _, err := f.ReadAt(body, pos+hdr); err != nil && err != io.EOF {
				return 0, 0, err
			}
			for i := 0; ; {
				j := bytes.Index(body[i:], []byte("tkhd"))
				if j < 0 {
					return 0, 0, errors.New("no video size found")
				}
				i += j
				// i is the box type; its size field starts 4 bytes earlier.
				off := i - 4
				ver := 0
				if i+4 < len(body) {
					ver = int(body[i+4])
				}
				wo := off + 84
				if ver == 1 {
					wo = off + 96
				}
				if off >= 0 && wo+8 <= len(body) {
					w := int(binary.BigEndian.Uint32(body[wo:])) >> 16
					hh := int(binary.BigEndian.Uint32(body[wo+4:])) >> 16
					if w > 0 && hh > 0 {
						return w, hh, nil
					}
				}
				i += 4
			}
		}
		pos += size
	}
}
