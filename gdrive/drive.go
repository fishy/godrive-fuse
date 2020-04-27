package gdrive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"sync/atomic"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

// Magic constants defined by Drive API.
const (
	// The magic id for root directory.
	RootID = "root"

	// The magic mime type for folders.
	FolderMimeType = "application/vnd.google-apps.folder"

	// The q string used for folders.
	FolderQString = `mimeType = '` + FolderMimeType + `'`

	// The q string used for non-folders.
	NotFolderQString = `mimeType != '` + FolderMimeType + `'`
)

// Default page size used by list calls.
const (
	PageSize = 50
)

// ErrBreak is an error can be used in ListFiles to break the list early.
var ErrBreak = errors.New("break list")

func splitPath(name string) []string {
	name = path.Clean(name)
	if name == "." || name == "/" {
		// Special cases
		return nil
	}
	return strings.Split(strings.Trim(name, "/"), "/")
}

// FindFile finds the file or directory on Drive by it's full path.
func (tc TracedClient) FindFile(ctx context.Context, name string, qStrings ...string) (string, error) {
	parts := splitPath(name)
	if len(parts) == 0 {
		return RootID, nil
	}
	return tc.findFileRecursive(ctx, RootID, parts, qStrings...)
}

func (tc TracedClient) findFileRecursive(ctx context.Context, parentID string, parts []string, addQ ...string) (string, error) {
	leaf := len(parts) <= 1

	name := parts[0]
	qStrings := []string{
		`name = '` + name + `'`,
	}
	if leaf {
		// Although this function is called -recursive,
		// addtional qStrings are only applied to leaves.
		qStrings = append(qStrings, addQ...)
	} else {
		qStrings = append(qStrings, FolderQString)
	}
	var foundID string
	err := tc.ListFiles(
		context.Background(),
		parentID,
		"files(id, name)",
		func(f *drive.File) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if f.Name == name {
				if leaf {
					// Recursion exit criteria
					foundID = f.Id
					return ErrBreak
				}
				id, err := tc.findFileRecursive(ctx, f.Id, parts[1:], addQ...)
				if id != "" && err == nil {
					foundID = id
					return ErrBreak
				}
				if err != nil {
					tc.Logger.Errorw(
						"findFileRecursive",
						"qStrings", qStrings,
						"err", err,
					)
				}
			}
			return nil
		},
		qStrings...,
	)
	if err == ErrBreak {
		return foundID, nil
	}
	return "", err
}

// ListFiles list all files under a directory.
func (tc TracedClient) ListFiles(
	ctx context.Context,
	parentID string,
	fields string,
	callback func(f *drive.File) error,
	qStrings ...string,
) error {
	list := tc.Files.List().PageSize(PageSize).Corpora("user").Fields(googleapi.Field(fields))
	list.OrderBy("folder,name")
	qStrings = append(qStrings, `'`+parentID+`' in parents`)
	qString := strings.Join(qStrings, ` and `)
	tc.Logger.Debugw("ListFiles", "qString", qString)
	list.Q(qString)
	var count uint64
	return list.Pages(
		ctx,
		func(l *drive.FileList) error {
			tc.Logger.Debugw(
				"ListFiles",
				"count", atomic.AddUint64(&count, uint64(len(l.Files))),
			)
			for _, f := range l.Files {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				if err := callback(f); err != nil {
					return err
				}
			}
			return nil
		},
	)
}

// DownloadByID downloads the file content by its id.
func (tc TracedClient) DownloadByID(ctx context.Context, id string) (*bytes.Buffer, error) {
	get := tc.Files.Get(id).Context(ctx)
	resp, err := get.Download()
	if err != nil {
		tc.Logger.Errorw(
			"DownloadByID",
			"err", err,
			"id", id,
		)
		return nil, err
	}
	defer resp.Body.Close()
	var buffer bytes.Buffer
	read, err := io.Copy(&buffer, resp.Body)
	if err != nil {
		tc.Logger.Errorw(
			"DownloadByID",
			"err", err,
			"id", id,
			"read", read,
		)
		return nil, err
	}
	tc.Logger.Debugw(
		"DownloadByID",
		"id", id,
		"read", read,
	)
	return &buffer, nil
}

// GetByID gets the file metadata by its id.
func (tc TracedClient) GetByID(ctx context.Context, id, fields string) (f *drive.File, err error) {
	get := tc.Files.Get(id).Context(ctx)
	f, err = get.Fields(googleapi.Field(fields)).Do()
	if err != nil {
		tc.Logger.Errorw(
			"GetByID",
			"err", err,
			"id", id,
		)
	}
	return
}

// UpdateMediaByID updates the file content by its id.
func (tc TracedClient) UpdateMediaByID(ctx context.Context, id string, r io.Reader) (f *drive.File, err error) {
	update := tc.Files.Update(id, nil).Context(ctx).Media(r, googleapi.ChunkSize(256*1024))
	f, err = update.Do()
	if err != nil {
		tc.Logger.Errorw(
			"UpdateMediaByID",
			"err", err,
			"id", id,
		)
	}
	return
}

// DeleteByID removes the given parent id from the file's parents list.
//
// Note that for directories this also deletes all its contents.
// It's caller's responsibility to ensure that it's empty.
func (tc TracedClient) DeleteByID(ctx context.Context, id, parentID string) (err error) {
	update := tc.Files.Update(id, nil).Context(ctx)
	update.RemoveParents(parentID)
	_, err = update.Do()
	if err != nil {
		tc.Logger.Errorw(
			"DeleteByID",
			"err", err,
			"id", id,
			"parentID", parentID,
		)
	}
	return
}

// Create creates a new file/directory under parent with given name.
func (tc TracedClient) Create(ctx context.Context, name, parentID string, isDir bool) (file *drive.File, err error) {
	file = &drive.File{
		Name:    name,
		Parents: []string{parentID},
	}
	if isDir {
		file.MimeType = FolderMimeType
	}
	create := tc.Files.Create(file).Context(ctx)
	if !isDir {
		create = create.Media(bytes.NewReader([]byte{}))
	}
	file, err = create.Do()
	if err != nil {
		tc.Logger.Errorw(
			"Create",
			"err", err,
			"name", name,
			"parentID", parentID,
			"isDir", isDir,
		)
	}
	return
}
