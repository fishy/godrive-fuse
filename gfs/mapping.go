package gfs

import (
	"hash/crc64"
)

var table = crc64.MakeTable(crc64.ECMA)

// IDtoInode maps a Drive id into an inode.
//
// It uses CRC64 ECMA table to do the mapping.
func IDtoInode(id string) uint64 {
	return crc64.Checksum([]byte(id), table)
}
