package gfs

import (
	"context"
	"os"
	"sync"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/reddit/baseplate.go/log"
	"github.com/reddit/baseplate.go/runtimebp"
	"go.uber.org/zap"
	"google.golang.org/api/drive/v3"

	"go.yhsif.com/godrive-fuse/gdrive"
)

// Mountpoints defines a mapping from local mount directory to Drive directory
// name.
type Mountpoints map[string]string

// Mountpoint defines a single mountpoint.
type Mountpoint struct {
	*fuse.Server

	Logger *zap.SugaredLogger
}

// Mount mounts the fs.
func Mount(tc gdrive.TracedClient, rootID string, to string) (*Mountpoint, error) {
	if err := os.MkdirAll(to, 0755); err != nil {
		return nil, err
	}
	root := &dirNode{
		commonNode: commonNode{
			id: rootID,
			tc: tc,
		},
	}
	server, err := fs.Mount(to, root, &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName: "godrive-fuse",
		},

		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
	})
	if err != nil {
		return nil, err
	}
	return &Mountpoint{
		Server: server,
		Logger: tc.Logger,
	}, nil
}

// MountAll mounts multiple mountpoints and blocks until they are all unmounted.
func MountAll(client *drive.Service, mounts Mountpoints) {
	var wg sync.WaitGroup
	servers := make([]*Mountpoint, 0, len(mounts))
	for to, dir := range mounts {
		to = os.ExpandEnv(to)
		logger := log.With(
			"from", dir,
			"to", to,
		)
		tc := gdrive.NewTracedClient(client, logger)
		id, err := tc.FindFile(context.Background(), dir, gdrive.FolderQString)
		if err != nil || id == "" {
			tc.Logger.Warnw("Unable to find mount_from, skipping...", "err", err)
			continue
		}
		if dir == "/" {
			tc.Logger.Errorw("Mounting root google drive currently not supported")
			continue
		}
		server, err := Mount(tc, id, to)
		if err != nil {
			tc.Logger.Errorw("Unable to mount", "err", err)
			continue
		}
		server.Logger.Info("Successfully mounted")
		servers = append(servers, server)
		wg.Add(1)
		go func(server *Mountpoint) {
			defer wg.Done()
			server.Wait()
			server.Logger.Info("Unmounted")
		}(server)
	}

	go runtimebp.HandleShutdown(
		context.Background(),
		func(sig os.Signal) {
			log.Infow("Unmounting all...", "signal", sig)
			var wg sync.WaitGroup
			for _, server := range servers {
				wg.Add(1)
				go func(server *Mountpoint) {
					defer wg.Done()
					if err := server.Unmount(); err != nil {
						server.Logger.Errorw("Unable to unmount", "err", err)
					}
				}(server)
			}
			wg.Wait()
		},
	)

	wg.Wait()
}
