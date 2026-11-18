package auth

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
)

// ReadValueByKeyMatch 从 SQLite 数据库中按键匹配读取 TEXT 值。
// 忠实移植自扩展的 sqlite-reader.js：支持 WAL 未回写页叠加、B-tree 遍历与溢出页链。
func ReadValueByKeyMatch(filePath string, match func(string) bool) (string, bool) {
	data, err := os.ReadFile(filePath)
	if err != nil || len(data) < 100 || string(data[:16]) != "SQLite format 3\x00" {
		return "", false
	}
	pageSize := int(binary.BigEndian.Uint16(data[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}
	reserved := int(data[20])
	usable := pageSize - reserved
	if pageSize <= 0 || usable <= 0 {
		return "", false
	}
	data = applyWAL(data, filePath+"-wal", pageSize)
	r := &sqliteReader{buf: data, pageSize: pageSize, usable: usable}

	rootVal := r.walk(1, func(cols []interface{}) interface{} {
		if len(cols) > 3 {
			if name, ok := cols[1].(string); ok && name == "ItemTable" {
				switch v := cols[3].(type) {
				case uint64:
					return int(v)
				case int:
					return v
				}
			}
		}
		return nil
	})
	rootPage, ok := rootVal.(int)
	if !ok || rootPage <= 0 {
		return "", false
	}
	val := r.walk(rootPage, func(cols []interface{}) interface{} {
		if len(cols) < 2 {
			return nil
		}
		key, ok := cols[0].(string)
		if !ok || !match(key) {
			return nil
		}
		value, ok := cols[1].(string)
		if !ok {
			return nil
		}
		return value
	})
	s, ok := val.(string)
	return s, ok
}

type sqliteReader struct {
	buf      []byte
	pageSize int
	usable   int
}

func (r *sqliteReader) page(no int) []byte {
	start := (no - 1) * r.pageSize
	if no <= 0 || start < 0 || start >= len(r.buf) {
		return nil
	}
	end := start + r.pageSize
	if end > len(r.buf) {
		end = len(r.buf)
	}
	return r.buf[start:end]
}

func (r *sqliteReader) headerOffset(no int) int {
	if no == 1 {
		return 100
	}
	return 0
}

func readVarint(b []byte, off int) (uint64, int) {
	var v uint64
	for i := 0; i < 9; i++ {
		if off+i >= len(b) {
			return 0, 1
		}
		cur := b[off+i]
		if i == 8 {
			return v*256 + uint64(cur), 9
		}
		v = v*128 + uint64(cur&0x7f)
		if cur&0x80 == 0 {
			return v, i + 1
		}
	}
	return v, 1
}

func (r *sqliteReader) payloadAt(pg []byte, cellOff int) []byte {
	size, n1 := readVarint(pg, cellOff)
	_, n2 := readVarint(pg, cellOff+n1)
	start := cellOff + n1 + n2
	total := int(size)
	maxLocal := r.usable - 35
	minLocal := ((r.usable-12)*32)/255 - 23
	localSize := total
	if total > maxLocal {
		localSize = minLocal + (total-minLocal)%(r.usable-4)
		if localSize > maxLocal {
			localSize = minLocal
		}
	}
	if start < 0 || localSize < 0 || start+localSize > len(pg) {
		return nil
	}
	local := append([]byte(nil), pg[start:start+localSize]...)
	if total <= maxLocal {
		return local
	}
	chunks := [][]byte{local}
	remaining := total - localSize
	next := uint64(0)
	if start+localSize+4 <= len(pg) {
		next = uint64(binary.BigEndian.Uint32(pg[start+localSize:]))
	}
	for next != 0 && remaining > 0 {
		p := r.page(int(next))
		if len(p) < 4 {
			break
		}
		take := r.usable - 4
		if take > remaining {
			take = remaining
		}
		if 4+take > len(p) {
			take = len(p) - 4
		}
		if take <= 0 {
			break
		}
		chunks = append(chunks, p[4:4+take])
		remaining -= take
		next = uint64(binary.BigEndian.Uint32(p[:4]))
	}
	return bytes.Join(chunks, nil)
}

func (r *sqliteReader) parseRecord(payload []byte) []interface{} {
	headerSize, n := readVarint(payload, 0)
	serials := []uint64{}
	off := n
	for off < int(headerSize) && off < len(payload) {
		v, l := readVarint(payload, off)
		serials = append(serials, v)
		off += l
	}
	intSizes := []int{0, 1, 2, 3, 4, 6, 8}
	out := make([]interface{}, 0, len(serials))
	vo := int(headerSize)
	for _, t := range serials {
		switch {
		case t == 0:
			out = append(out, nil)
		case t >= 1 && t <= 6:
			size := intSizes[t]
			var v uint64
			for i := 0; i < size && vo+i < len(payload); i++ {
				v = v*256 + uint64(payload[vo+i])
			}
			out = append(out, v)
			vo += size
		case t == 7:
			var v float64
			if vo+8 <= len(payload) {
				v = math.Float64frombits(binary.BigEndian.Uint64(payload[vo : vo+8]))
			}
			out = append(out, v)
			vo += 8
		case t == 8:
			out = append(out, uint64(0))
		case t == 9:
			out = append(out, uint64(1))
		case t >= 12:
			n := 0
			if t%2 == 0 {
				n = int((t - 12) / 2)
			} else {
				n = int((t - 13) / 2)
			}
			if vo+n > len(payload) {
				n = len(payload) - vo
			}
			if n < 0 {
				n = 0
			}
			out = append(out, string(payload[vo:vo+n]))
			vo += n
		default:
			out = append(out, nil)
		}
	}
	return out
}

func (r *sqliteReader) walk(pageNo int, visit func([]interface{}) interface{}) interface{} {
	pg := r.page(pageNo)
	h := r.headerOffset(pageNo)
	if len(pg) < h+8 {
		return nil
	}
	pgType := pg[h]
	if h+3+2 > len(pg) {
		return nil
	}
	numCells := int(binary.BigEndian.Uint16(pg[h+3 : h+5]))
	switch pgType {
	case 13: // table leaf
		for i := 0; i < numCells; i++ {
			ptr := h + 8 + i*2
			if ptr+2 > len(pg) {
				continue
			}
			cellOff := int(binary.BigEndian.Uint16(pg[ptr : ptr+2]))
			payload := r.payloadAt(pg, cellOff)
			if payload == nil {
				continue
			}
			if res := visit(r.parseRecord(payload)); res != nil {
				return res
			}
		}
	case 5: // table interior
		for i := 0; i < numCells; i++ {
			ptr := h + 12 + i*2
			if ptr+2 > len(pg) {
				continue
			}
			cellOff := int(binary.BigEndian.Uint16(pg[ptr : ptr+2]))
			if cellOff+4 > len(pg) {
				continue
			}
			child := int(binary.BigEndian.Uint32(pg[cellOff : cellOff+4]))
			if res := r.walk(child, visit); res != nil {
				return res
			}
		}
		if h+12 <= len(pg) {
			return r.walk(int(binary.BigEndian.Uint32(pg[h+8:h+12])), visit)
		}
	}
	return nil
}

type walFrame struct {
	pageNo int
	offset int
}

func applyWAL(db []byte, walPath string, dbPageSize int) []byte {
	wal, err := os.ReadFile(walPath)
	if err != nil || len(wal) < 32 {
		return db
	}
	magic := binary.BigEndian.Uint32(wal[0:4])
	if magic != 0x377f0682 && magic != 0x377f0683 {
		return db
	}
	pageSize := int(binary.BigEndian.Uint32(wal[8:12]))
	if pageSize != dbPageSize {
		return db
	}
	salt1 := binary.BigEndian.Uint32(wal[16:20])
	salt2 := binary.BigEndian.Uint32(wal[20:24])
	frames := []walFrame{}
	committed := 0
	for off := 32; off+24+pageSize <= len(wal); off += 24 + pageSize {
		pageNo := int(binary.BigEndian.Uint32(wal[off : off+4]))
		dbSize := binary.BigEndian.Uint32(wal[off+4 : off+8])
		if binary.BigEndian.Uint32(wal[off+8:off+12]) != salt1 ||
			binary.BigEndian.Uint32(wal[off+12:off+16]) != salt2 {
			break
		}
		if pageNo == 0 {
			break
		}
		frames = append(frames, walFrame{pageNo: pageNo, offset: off + 24})
		if dbSize != 0 {
			committed = len(frames)
		}
	}
	if committed == 0 {
		return db
	}
	maxPage := 0
	for i := 0; i < committed; i++ {
		if frames[i].pageNo > maxPage {
			maxPage = frames[i].pageNo
		}
	}
	needed := maxPage * pageSize
	var img []byte
	if len(db) < needed {
		img = make([]byte, needed)
		copy(img, db)
	} else {
		img = append([]byte(nil), db...)
	}
	for i := 0; i < committed; i++ {
		f := frames[i]
		start := (f.pageNo - 1) * pageSize
		if start >= 0 && start+pageSize <= len(img) && f.offset+pageSize <= len(wal) {
			copy(img[start:start+pageSize], wal[f.offset:f.offset+pageSize])
		}
	}
	return img
}
