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

	switch cmd {
	default:
		flag.Usage()
		return
	case "mount":
		gfs.MountAll(srv, gfs.Mountpoints{flag.Arg(2): flag.Arg(1)})
	}
}