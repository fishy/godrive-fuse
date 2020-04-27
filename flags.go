package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Flags.
var (
	configDir = flag.String(
		"config-dir",
		getDefaultConfigDir(),
		fmt.Sprintf(
			`The directory to config files, by default $XDG_CONFIG_HOME/%s will be used`,
			ConfigSubDir,
		),
	)
	profile = flag.String(
		"profile",
		"default",
		"If you have more than one google account, use this to contrrol which account to use",
	)
	dummy = flag.Bool(
		"dummy",
		false,
		"dummy flag",
	)
)

// ConfigSubDir is the subdir under root config directory.
const ConfigSubDir = "godrive-fuse"

func getDefaultConfigDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir != "" {
		return filepath.Join(configDir, ConfigSubDir)
	}
	return filepath.Join(os.Getenv("HOME"), ".config", ConfigSubDir)
}

func setFlagUsage() {
	flag.Usage = func() {
		fmt.Fprintf(
			flag.CommandLine.Output(),
			`Usage:
	%s [args] command [command args]

Commands:
  help:
	Show this message.

  init:
	Initialize the config file before first use.

  mount <drive-directory> <local-directory>:
	Mount the specified Drive directory to the local directory.

Args:
`,
			os.Args[0],
		)
		flag.PrintDefaults()
	}
}
