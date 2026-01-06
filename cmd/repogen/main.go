package main

import (
	"os"

	"github.com/ralt/repogen/internal/cli"
	"github.com/sirupsen/logrus"
)

func main() {
	// Setup logging format
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	rootCmd := cli.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}
