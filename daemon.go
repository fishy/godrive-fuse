package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reddit/baseplate.go/log"
	daemon "gopkg.in/sevlyar/go-daemon.v0"
)

const layout = time.RFC3339

// DaemonConfig defines the configurations needed by the daemon.
type DaemonConfig struct {
	Dir         string `yaml:"dir"`
	CleanupDays int    `yaml:"cleanup_days"`
}

func getDefaultDaemonDir() string {
	daemonDir := os.Getenv("XDG_DATA_HOME")
	if daemonDir != "" {
		return filepath.Join(daemonDir, ConfigSubDir)
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", ConfigSubDir)
}

func runDaemon(cfg DaemonConfig) (child bool, d *daemon.Context) {
	if cfg.Dir == "" {
		cfg.Dir = getDefaultDaemonDir()
	}
	cfg.Dir = os.ExpandEnv(cfg.Dir)
	cleanupDaemonFiles(cfg)
	if *noDaemon {
		return true, nil
	}
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		log.Fatalw("Unable to prepare daemon directory", "dir", cfg.Dir, "err", err)
	}
	file := time.Now().UTC().Format(layout)
	d = &daemon.Context{
		PidFileName: filepath.Join(cfg.Dir, file+".pid"),
		PidFilePerm: 0644,
		LogFileName: filepath.Join(cfg.Dir, file+".log"),
		LogFilePerm: 0644,
	}
	cproc, err := d.Reborn()
	if err != nil {
		log.Fatalw("Unable to fork daemon", "err", err)
	}
	return cproc == nil, d
}

func cleanupDaemonFiles(cfg DaemonConfig) {
	threshold := time.Now().Add(time.Hour * -24 * time.Duration(cfg.CleanupDays))
	if err := filepath.Walk(
		cfg.Dir,
		func(path string, info os.FileInfo, err error) error {
			log.Debugw("WalkFunc", "path", path)
			_, file := filepath.Split(path)
			if err != nil {
				if !os.IsNotExist(err) {
					log.Errorw("Skipping file", "file", file, "err", err)
				}
				return nil
			}
			if info.IsDir() {
				if path == cfg.Dir {
					return nil
				}
				log.Debugw("Skipping directory", "dir", file)
				return filepath.SkipDir
			}
			parts := strings.Split(file, ".")
			if len(parts) != 2 {
				log.Debugw("Skipping unrelated file", "file", file)
				return nil
			}
			base, ext := parts[0], parts[1]
			switch ext {
			default:
				log.Debugw("Skipping unrelated file", "file", file)
			case "pid", "log":
				t, err := time.Parse(layout, base)
				if err != nil {
					log.Debugw("Skipping unrelated file", "file", file)
					return nil
				}
				if t.After(threshold) {
					return nil
				}
				if err := os.Remove(path); err != nil {
					log.Errorw("Failed to delete old daemon file", "file", file, "err", err)
				} else {
					log.Infow("Cleaned up old daemon file", "file", file)
				}
			}
			return nil
		},
	); err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Errorw("Unable to cleanup daemon files", "dir", cfg.Dir, "err", err)
	}
}
