package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	collector "github.com/banyanops/collector"
	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
	flag "github.com/docker/docker/pkg/mflag"
)

func init() {
	config.DefineDestsFlag("file")
	config.BanyanUpdate = func(status ...string) {}
}

func checkConfigUpdate(initial bool) (updates bool) {
	return checkRepoList(initial)
}

// doFlags defines the cmdline Usage string and parses flag options.
func doFlags() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Usage: %s [OPTIONS] REGISTRY REPO [REPO...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  REGISTRY:\n")
		fmt.Fprintf(os.Stderr, "\tURL of your Docker registry; use index.docker.io for Docker Hub \n")
		fmt.Fprintf(os.Stderr, "\n  REPO:\n")
		fmt.Fprintf(os.Stderr, "\tOne or more repos to gather info about; if no repo is specified Collector will gather info on *all* repos in the Registry\n")
		fmt.Fprintf(os.Stderr, "\n  Environment variables:\n")
		fmt.Fprintf(os.Stderr, "\tCOLLECTOR_DIR:   (Required) Directory that contains the \"data\" folder with Collector default scripts, e.g., $GOPATH/src/github.com/banyanops/collector\n")
		fmt.Fprintf(os.Stderr, "\tCOLLECTOR_ID:    ID provided by Banyan web interface to register Collector with the Banyan service\n")
		fmt.Fprintf(os.Stderr, "\tBANYAN_HOST_DIR: Host directory mounted into Collector/Target containers where results are stored (default: $HOME/.banyan)\n")
		fmt.Fprintf(os.Stderr, "\tBANYAN_DIR:      (Specify only in Dockerfile) Directory in the Collector container where host directory BANYAN_HOST_DIR is mounted\n")
		fmt.Fprintf(os.Stderr, "\tDOCKER_{HOST,CERT_PATH,TLS_VERIFY}: If set, e.g., by docker-machine, then they take precedence over --dockerProto and --dockerAddr\n")
		printExampleUsage()
		fmt.Fprintf(os.Stderr, "  Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if config.COLLECTORDIR() == "" {
		flag.Usage()
		os.Exit(1)
	}
	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}
	if *dockerProto != "unix" && *dockerProto != "tcp" {
		flag.Usage()
		os.Exit(1)
	}
	requiredDirs := []string{config.BANYANDIR(), filepath.Dir(*imageList), filepath.Dir(*repoList), *config.BanyanOutDir, collector.DefaultScriptsDir, collector.UserScriptsDir, collector.BinDir}
	for _, dir := range requiredDirs {
		blog.Debug("Creating directory: " + dir)
		err := collector.CreateDirIfNotExist(dir)
		if err != nil {
			blog.Exit(err, ": Error in creating a required directory: ", dir)
		}
	}
	collector.RegistrySpec = flag.Arg(0)
	//nextMaxImages = *maxImages
}

func SetOutputWriters(authToken string) {
	dests := strings.Split(*config.Dests, ",")
	for _, dest := range dests {
		var writer collector.Writer
		switch dest {
		case "file":
			writer = collector.NewFileWriter("json", *config.BanyanOutDir)
		default:
			blog.Error("No such output writer!")
			//ignore the rest and keep going
			continue
		}
		collector.WriterList = append(collector.WriterList, writer)
	}
}

func RegisterCollector() (token string) {
	return ""
}

func SetupBanyanStatus(authToken string) {
}
