package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/klauspost/compress/zstd"
	P "github.com/metacubex/mihomo/constant/provider"
	"io"
)

// Validate lengths and tree topology before the upstream reader allocates from
// a downloaded MRS file. Bundled files are fixed by their build-time checksums.
func validateMRS(data []byte, b P.RuleBehavior) (int, error) {
	bad := errors.New("MRS 结构或大小无效")
	z, err := zstd.NewReader(bytes.NewReader(data), zstd.WithDecoderMaxMemory(16<<20), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return 0, bad
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, 8<<20+1))
	if err != nil || len(raw) < 22 || len(raw) > 8<<20 || string(raw[:4]) != "MRS\x01" || raw[4] != b.Byte() {
		return 0, bad
	}
	count := binary.BigEndian.Uint64(raw[5:13])
	extra := binary.BigEndian.Uint64(raw[13:21])
	if count == 0 || count > 1000000 || extra >= uint64(len(raw)-21) {
		return 0, bad
	}
	buf := raw[21+int(extra):]
	if buf[0] != 1 {
		return 0, bad
	}
	buf = buf[1:]
	vector := func(size int) []byte {
		if len(buf) < 8 {
			return nil
		}
		n := binary.BigEndian.Uint64(buf[:8])
		buf = buf[8:]
		if n == 0 || n > uint64(len(buf)/size) {
			return nil
		}
		v := buf[:int(n)*size]
		buf = buf[len(v):]
		return v
	}
	if b == P.IPCIDR {
		if vector(32) == nil || len(buf) != 0 {
			return 0, bad
		}
		return int(count), nil
	}
	if b != P.Domain {
		return 0, bad
	}
	leaves, bits, labels := vector(8), vector(8), vector(1)
	if leaves == nil || bits == nil || labels == nil || len(buf) != 0 {
		return 0, bad
	}
	nodes := len(labels) + 1
	if len(leaves)*8 < nodes || len(bits)*8 < 2*nodes-1 {
		return 0, bad
	}
	edges, ends := 0, 0
	for i := 0; i < 2*nodes-1; i++ {
		v := binary.BigEndian.Uint64(bits[(i/64)*8:])
		if v&(1<<uint(i%64)) != 0 {
			ends++
		} else {
			edges++
		}
		if ends > edges && i != 2*nodes-2 {
			return 0, bad
		}
	}
	if ends != nodes || edges != len(labels) {
		return 0, bad
	}
	return int(count), nil
}
