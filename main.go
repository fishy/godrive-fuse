package main

import (
	"flag"

	"github.com/reddit/baseplate.go/log"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"

	"github.com/fishy/godrive-fuse/gfs"
)

func main() {
	setFlagUsage()
	flag.Parse()

	cmd := flag.Arg(0)
	switch cmd {
	case "help":
		flag.Usage()
		return
	case "init":
		InitConfigFile(*configDir)
		return
	}

	cfg, _ := ParseConfigFromDir(*configDir)

	log.InitLogger(cfg.LogLevel)
	defer log.Sync()

	client := GetOAuthClient(
		getClientContext(context.Background(), cfg.HTTPClient),
		Args{
			Directory: *configDir,
			Profile:   *profile,
		},
		cfg.OAuthClient,
	)
	srv, err := drive.New(client)
	if err != nil {
		log.Fatalw("Unable to retrieve Drive client", "err", err)
	}

	var mountpoints gfs.Mountpoints
	if flag.Arg(1) != "" {
		if flag.Arg(2) != "" {
			mountpoints = gfs.Mountpoints{flag.Arg(2): flag.Arg(1)}
		} else {
			mountpoints = gfs.Mountpoints{flag.Arg(1): "/"}
		}
	} else {
		mountpoints = cfg.Mountpoints
	}

	switch cmd {
	default:
		flag.Usage()
		return
	case "mount":
		child, d := runDaemon(cfg.Daemon)
		if child {
			if d != nil {
				defer d.Release()
			}
			gfs.MountAll(srv, mountpoints)
		}
	}
}
