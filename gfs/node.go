package gfs

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	lru "github.com/hashicorp/golang-lru"
	"google.golang.org/api/drive/v3"

	"go.yhsif.com/godrive-fuse/gdrive"
)

// The number of entries for the files cache.
const (
	LRUSize = 1000
)

const (
	filesFields = "files(id, name, mimeType, size, createdTime, modifiedTime)"
)

// global id -> filesCacheEntry cache
var globalFilesCache *lru.TwoQueueCache

func init() {
	var err error
	globalFilesCache, err = lru.New2Q(LRUSize)
	if err != nil {
		panic(err)
	}
}

type commonNode struct {
	fs.Inode

	id string
	tc gdrive.TracedClient
}

func (cn *commonNode) parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		cn.tc.Logger.Warnw(
			"unable to parse time",
			"err", err,
			"time", s,
		)
		t = time.Now()
	}
	return &t
}

func (cn *commonNode) cacheFile(f *drive.File) *filesCacheEntry {
	var isDir bool
	var mode uint32 = fuse.S_IFREG
	if f.MimeType == gdrive.FolderMimeType {
		mode = fuse.S_IFDIR
		isDir = true
	}
	entry := &filesCacheEntry{
		name:   f.Name,
		id:     f.Id,
		isDir:  isDir,
		cached: time.Now(),
		ino:    IDtoInode(f.Id),
		mode:   mode,
		size:   f.Size,
		ctime:  cn.parseTime(f.CreatedTime),
		mtime:  cn.parseTime(f.ModifiedTime),
	}
	globalFilesCache.Add(f.Id, entry)
	return entry
}

type filesCacheEntry struct {
	// key fields
	name   string
	id     string
	isDir  bool
	cached time.Time

	// dir entry needed fields
	ino  uint64
	mode uint32

	// attr needed fields
	size  int64
	ctime *time.Time
	mtime *time.Time
}

func (e filesCacheEntry) ToDirEntry() fuse.DirEntry {
	return fuse.DirEntry{
		Name: e.name,
		Ino:  e.ino,
		Mode: e.mode,
	}
}

func (e filesCacheEntry) SetAttr(out *fuse.Attr) {
	out.Ino = e.ino
	out.Size = uint64(e.size)
	out.SetTimes(nil, e.mtime, e.ctime)
	out.Owner.Uid = uint32(syscall.Getuid())
	out.Owner.Gid = uint32(syscall.Getgid())
}

type dirNode struct {
	commonNode

	filesCache sync.Map
}

var (
	_ fs.NodeLookuper  = (*dirNode)(nil)
	_ fs.NodeReaddirer = (*dirNode)(nil)
	_ fs.NodeUnlinker  = (*dirNode)(nil)
	_ fs.NodeRmdirer   = (*dirNode)(nil)
	_ fs.NodeCreater   = (*dirNode)(nil)
	_ fs.NodeMkdirer   = (*dirNode)(nil)
)

func (dn *dirNode) loadCache(ctx context.Context, name string) (entry *filesCacheEntry) {
	if value, ok := dn.filesCache.Load(name); ok {
		if entry, ok := value.(*filesCacheEntry); ok {
			return entry
		}
	}
	err := dn.commonNode.tc.NewChild().ListFiles(
		ctx,
		dn.id,
		filesFields,
		func(f *drive.File) error {
			entry = dn.cacheFile(f)
			return nil
		},
		`name = '`+name+`'`,
	)
	if err != nil {
		dn.commonNode.tc.Logger.Warnw(
			"ListFiles failed",
			"err", err,
		)
		return nil
	}
	if entry == nil {
		return nil
	}
	return
}

func (dn *dirNode) cacheFile(f *drive.File) *filesCacheEntry {
	entry := dn.commonNode.cacheFile(f)
	if !entry.isDir {
		dn.filesCache.Store(f.Name, entry)
	}
	return entry
}

func (dn *dirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	var lock sync.Mutex
	var files []fuse.DirEntry
	err := dn.commonNode.tc.NewChild().ListFiles(
		ctx,
		dn.id,
		filesFields,
		func(f *drive.File) error {
			entry := dn.cacheFile(f)

			lock.Lock()
			defer lock.Unlock()
			files = append(files, entry.ToDirEntry())
			return nil
		},
	)
	if err != nil {
		dn.commonNode.tc.Logger.Errorw(
			"ListFiles failed",
			"err", err,
		)
		return fs.NewListDirStream(files), syscall.ECANCELED
	}
	return fs.NewListDirStream(files), 0
}

func (dn *dirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	dn.commonNode.tc.Logger.Debugw(
		"Lookup called",
		"id", dn.commonNode.id,
		"name", name,
	)

	entry := dn.loadCache(ctx, name)
	if entry == nil {
		return nil, syscall.ENOENT
	}

	attr := fs.StableAttr{
		Mode: entry.mode,
		Ino:  entry.ino,
	}
	var node fs.InodeEmbedder
	if entry.isDir {
		node = &dirNode{
			commonNode: commonNode{
				id: entry.id,
				tc: dn.commonNode.tc,
			},
		}
	} else {
		node = &fileNode{
			commonNode: commonNode{
				id: entry.id,
				tc: dn.commonNode.tc,
			},
			entry: entry,
		}
	}
	child := dn.NewInode(ctx, node, attr)
	entry.SetAttr(&out.Attr)
	return child, 0
}

func (dn *dirNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	dn.commonNode.tc.Logger.Debugw(
		"Mkdir called",
		"id", dn.commonNode.id,
		"name", name,
		"mode", mode,
	)

	entry := dn.loadCache(ctx, name)
	if entry != nil {
		return nil, syscall.EEXIST
	}

	file, err := dn.commonNode.tc.NewChild().Create(
		ctx,
		name,
		dn.commonNode.id,
		true, // isDir
	)
	if err != nil {
		return nil, syscall.EREMOTEIO
	}
	entry = dn.cacheFile(file)

	attr := fs.StableAttr{
		Mode: entry.mode,
		Ino:  entry.ino,
	}
	node := &dirNode{
		commonNode: commonNode{
			id: entry.id,
			tc: dn.commonNode.tc,
		},
	}
	child := dn.NewInode(ctx, node, attr)
	entry.SetAttr(&out.Attr)
	return child, 0
}

func (dn *dirNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	dn.commonNode.tc.Logger.Debugw(
		"Create called",
		"id", dn.commonNode.id,
		"name", name,
		"flags", flags,
		"mode", mode,
	)

	entry := dn.loadCache(ctx, name)
	if entry != nil {
		errno = syscall.EEXIST
		return
	}

	file, err := dn.commonNode.tc.NewChild().Create(
		ctx,
		name,
		dn.commonNode.id,
		false, // isDir
	)
	if err != nil {
		errno = syscall.EREMOTEIO
		return
	}
	entry = dn.cacheFile(file)

	attr := fs.StableAttr{
		Mode: entry.mode,
		Ino:  entry.ino,
	}
	embedder := &fileNode{
		commonNode: commonNode{
			id: entry.id,
			tc: dn.commonNode.tc,
		},
		entry:  entry,
		buffer: new(bytes.Buffer),
	}
	fh = embedder
	node = dn.NewInode(ctx, embedder, attr)
	entry.SetAttr(&out.Attr)
	return
}

func (dn *dirNode) Unlink(ctx context.Context, name string) syscall.Errno {
	entry := dn.loadCache(ctx, name)
	if entry == nil {
		return syscall.ENOENT
	}
	if entry.isDir {
		return syscall.ENOTSUP
	}
	err := dn.commonNode.tc.NewChild().DeleteByID(ctx, entry.id, dn.commonNode.id)
	if err != nil {
		return syscall.EREMOTEIO
	}
	dn.filesCache.Delete(name)
	globalFilesCache.Remove(entry.id)
	return 0
}

func (dn *dirNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	entry := dn.loadCache(ctx, name)
	if entry == nil {
		return syscall.ENOENT
	}
	if !entry.isDir {
		return syscall.ENOTSUP
	}
	newNode := &dirNode{
		commonNode: commonNode{
			id: entry.id,
			tc: dn.commonNode.tc,
		},
	}
	_, errno := newNode.Readdir(ctx)
	if errno != 0 {
		return errno
	}
	var found bool
	newNode.filesCache.Range(func(k, v interface{}) bool {
		// We expect the map to be empty. Fail it on the first call
		found = true
		return false
	})
	if found {
		return syscall.ENOTSUP
	}
	err := dn.commonNode.tc.NewChild().DeleteByID(ctx, entry.id, dn.commonNode.id)
	if err != nil {
		return syscall.EREMOTEIO
	}
	dn.filesCache.Delete(name)
	globalFilesCache.Remove(entry.id)
	return 0
}

type fileNode struct {
	commonNode

	lock   sync.Mutex
	entry  *filesCacheEntry
	buffer *bytes.Buffer
}

var (
	_ fs.NodeOpener    = (*fileNode)(nil)
	_ fs.NodeGetattrer = (*fileNode)(nil)
	_ fs.NodeSetattrer = (*fileNode)(nil)
	_ fs.FileReader    = (*fileNode)(nil)
	_ fs.FileWriter    = (*fileNode)(nil)
	_ fs.FileFlusher   = (*fileNode)(nil)
)

func (fn *fileNode) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	fn.commonNode.tc.Logger.Debugw(
		"Open called",
		"id", fn.commonNode.id,
		"flags", flags,
	)
	return fn, flags, 0
}

func (fn *fileNode) Getattr(ctx context.Context, _ fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fn.commonNode.tc.Logger.Debugw(
		"Getattr called",
		"id", fn.commonNode.id,
	)

	fn.loadCache(ctx)
	if fn.entry == nil {
		return syscall.ENOENT
	}
	fn.entry.SetAttr(&out.Attr)
	return 0
}

func (fn *fileNode) Setattr(ctx context.Context, _ fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	fn.commonNode.tc.Logger.Debugw(
		"Setattr called",
		"id", fn.commonNode.id,
		"in", *in,
	)

	fn.lock.Lock()
	defer fn.lock.Unlock()

	if size, ok := in.GetSize(); ok {
		fn.resize(ctx, int(size))
		if fn.buffer == nil {
			return syscall.EREMOTEIO
		}
	}

	fn.loadCache(ctx)
	if fn.entry == nil {
		return syscall.EREMOTEIO
	}
	fn.entry.SetAttr(&out.Attr)
	return 0
}

func (fn *fileNode) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fn.commonNode.tc.Logger.Debugw(
		"Read called",
		"id", fn.commonNode.id,
		"buf size", len(dest),
		"off", off,
	)

	fn.lock.Lock()
	defer fn.lock.Unlock()

	fn.loadBuffer(ctx)
	if fn.buffer == nil {
		return nil, syscall.ENOENT
	}

	var size int
	if off < int64(fn.buffer.Len()) {
		size = fn.buffer.Len() - int(off)
		copy(dest, fn.buffer.Bytes()[off:])
		if size > len(dest) {
			size = len(dest)
		}
	}
	return fuse.ReadResultData(dest[:size]), 0
}

func (fn *fileNode) resize(ctx context.Context, size int) {
	defer func() {
		if fn.buffer != nil {
			fn.loadCache(ctx)
			if fn.entry != nil {
				fn.entry.size = int64(size)
			}
		}
	}()

	if size == 0 {
		fn.buffer = new(bytes.Buffer)
		return
	}

	fn.loadBuffer(ctx)
	if fn.buffer == nil {
		return
	}
	if size < fn.buffer.Len() {
		fn.buffer.Truncate(size)
		return
	}
	if size > fn.buffer.Len() {
		fn.buffer.ReadFrom(io.LimitReader(nullReader{}, int64(size-fn.buffer.Len())))
		return
	}
}

func (fn *fileNode) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	fn.commonNode.tc.Logger.Debugw(
		"Write called",
		"id", fn.commonNode.id,
		"data size", len(data),
		"off", off,
	)

	fn.lock.Lock()
	defer fn.lock.Unlock()

	fn.resize(ctx, int(off))
	if fn.buffer == nil {
		return 0, syscall.ENOENT
	}
	n, _ := fn.buffer.Write(data)
	fn.loadCache(ctx)
	if fn.entry != nil {
		fn.entry.size = off + int64(n)
	}
	return uint32(n), 0
}

func (fn *fileNode) Flush(ctx context.Context) syscall.Errno {
	fn.commonNode.tc.Logger.Debugw(
		"Flush called",
		"id", fn.commonNode.id,
	)

	fn.lock.Lock()
	defer fn.lock.Unlock()

	f, err := fn.commonNode.tc.NewChild().UpdateMediaByID(
		ctx,
		fn.commonNode.id,
		strings.NewReader(fn.buffer.String()),
	)
	if err != nil {
		return syscall.EREMOTEIO
	}
	fn.cacheFile(f)
	return 0
}

func (fn *fileNode) loadCache(ctx context.Context) {
	if fn.entry != nil {
		return
	}
	if value, ok := globalFilesCache.Get(fn.commonNode.id); ok {
		if entry, ok := value.(*filesCacheEntry); ok {
			fn.entry = entry
			return
		}
	}
	f, _ := fn.commonNode.tc.NewChild().GetByID(ctx, fn.commonNode.id, filesFields)
	if f != nil {
		fn.entry = fn.cacheFile(f)
	}
}

func (fn *fileNode) loadBuffer(ctx context.Context) {
	if fn.buffer != nil {
		return
	}
	buffer, err := fn.commonNode.tc.NewChild().DownloadByID(ctx, fn.commonNode.id)
	if err == nil {
		fn.buffer = buffer
		globalFilesCache.Add(fn.commonNode.id, buffer)
	}
	return
}

type nullReader struct{}

func (nullReader) Read(p []byte) (int, error) {
	return len(p), nil
}

var _ io.Reader = nullReader{}
