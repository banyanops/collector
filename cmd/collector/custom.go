package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	collector "github.com/banyanops/collector"
	auth "github.com/banyanops/collector/auth"
	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
	fsutil "github.com/banyanops/collector/fsutil"
	blog "github.com/ccpaging/log4go"
	flag "github.com/spf13/pflag"
)

func init() {
	config.DefineDestsFlag("file")
	config.BanyanUpdate = func(status ...string) {}
}

func initMetadataSet(tokenSync *auth.TokenSyncInfo, metadataSet collector.MetadataSet) {
	return
}

func checkConfigUpdate(initial bool) (updates bool) {
	return checkRepoList(initial)
}

// doFlags defines the cmdline Usage string and parses flag options.
func doFlags() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Usage: %s [OPTIONS] REGISTRY REPO [REPO...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  REGISTRY:\n")
		fmt.Fprintf(os.Stderr, "\tURL of your Docker registry; use "+config.DockerHub+" for Docker Hub, use local.host to collect images from local Docker host\n")
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
		os.Exit(except.ErrorExitStatus)
	}
	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(except.ErrorExitStatus)
	}
	if *dockerProto != "unix" && *dockerProto != "tcp" {
		flag.Usage()
		os.Exit(except.ErrorExitStatus)
	}
	requiredDirs := []string{config.BANYANDIR(), filepath.Dir(*imageList), filepath.Dir(*repoList), *config.BanyanOutDir, collector.DefaultScriptsDir, collector.UserScriptsDir, collector.BinDir}
	for _, dir := range requiredDirs {
		blog.Debug("Creating directory: " + dir)
		err := fsutil.CreateDirIfNotExist(dir)
		if err != nil {
			except.Fail(err, ": Error in creating a required directory: ", dir)
		}
	}
	collector.RegistrySpec = flag.Arg(0)
	// EqualFold: case insensitive comparison
	if strings.EqualFold(flag.Arg(0), "local.host") {
		collector.LocalHost = true
	}
	//nextMaxImages = *maxImages

	if *maxRequests != 0 {
		err := collector.AddRegistryRateLimiter(*maxRequests, *timePeriod)
		if err != nil {
			except.Fail(err, ": Error in setting registry rate limiter")
		}
	}
	if *maxRequests2 != 0 {
		err := collector.AddRegistryRateLimiter(*maxRequests2, *timePeriod2)
		if err != nil {
			except.Fail(err, ": Error in setting registry rate limiter")
		}
	}
}

func printExampleUsage() {
	fmt.Fprintf(os.Stderr, "\n  Examples:\n")
	fmt.Fprintf(os.Stderr, "  (a) Running when compiled from source (standalone mode):\n")
	fmt.Fprintf(os.Stderr, "  \tcd <COLLECTOR_SOURCE_DIR>\n")
	fmt.Fprintf(os.Stderr, "  \tsudo COLLECTOR_DIR=$PWD $GOPATH/bin/collector "+config.DockerHub+" banyanops/nginx\n\n")
	fmt.Fprintf(os.Stderr, "  (b) Running inside a Docker container: \n")
	fmt.Fprintf(os.Stderr, "  \tsudo docker run --rm \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v ~/.docker:/root/.docker \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v ~/.dockercfg:/root/.dockercfg \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v /var/run/docker.sock:/var/run/docker.sock \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v $HOME/.banyan:/banyandir \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v <USER_SCRIPTS_DIR>:/banyancollector/data/userscripts \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-e BANYAN_HOST_DIR=$HOME/.banyan \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\tbanyanops/collector "+config.DockerHub+" banyanops/nginx\n\n")
}

func SetOutputWriters(tokenSync *auth.TokenSyncInfo) {
	dests := strings.Split(*config.Dests, ",")
	for _, dest := range dests {
		var writer collector.Writer
		switch dest {
		case "file":
			writer = collector.NewFileWriter("json", *config.BanyanOutDir)
		default:
			except.Error("No such output writer!")
			//ignore the rest and keep going
			continue
		}
		collector.WriterList = append(collector.WriterList, writer)
	}
}

func RegisterCollector(tokenSync *auth.TokenSyncInfo) {
	tokenSync.UpdateToken("")
	return
}

func SetupBanyanStatus(tokenSync *auth.TokenSyncInfo) {
}
