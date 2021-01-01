package gfs_test

import (
	"hash/crc64"
	"testing"

	"github.com/reddit/baseplate.go/randbp"

	"go.yhsif.com/godrive-fuse/gdrive"
)

func BenchmarkCRC64(b *testing.B) {
	tables := map[string]*crc64.Table{
		"ISO":  crc64.MakeTable(crc64.ISO),
		"ECMA": crc64.MakeTable(crc64.ECMA),
	}
	ids := []string{
		gdrive.RootID,
		"1-3pxPSAQG8Sk9GJigM8E1M24VtV1ilhZ",
		"1ptgtbuoGn_ypmSBIN5eqncvxGZrgKVhA",
		"1bzXmbfRhainTOHryPfWKGrlvqFLD8_vw",
		"1kGxb29wbSiSshUSS92iv5flzyaEG9hJm",
	}
	for label, table := range tables {
		b.Run(
			label,
			func(b *testing.B) {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						id := ids[randbp.R.Intn(len(ids))]
						crc64.Checksum([]byte(id), table)
					}
				})
			},
		)
	}
}
