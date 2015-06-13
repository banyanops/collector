package main

// Collector is a program that extracts static information from container images stored in a Docker registry.

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	collector "github.com/banyanops/collector"
	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
	flag "github.com/docker/docker/pkg/mflag"
)

const (
	// Console logging level
	CONSOLELOGLEVEL = blog.INFO
	// File logging level
	FILELOGLEVEL = blog.FINEST
	// Number of docker images to process in a single batch.
	IMAGEBATCH = 5
)

var (
	LOGFILENAME = config.BANYANDIR() + "/hostcollector/collector.log"
	imageList   = flag.String([]string{"#-imagelist"}, config.BANYANDIR()+"/hostcollector/imagelist",
		"List of previously collected images (file)")
	repoList = flag.String([]string{"r", "-repolist"}, config.BANYANDIR()+"/hostcollector/repolist",
		"File containing list of repos to process")

	// Configuration parameters for speed/efficiency
	removeThresh = flag.Int([]string{"-removethresh"}, 10,
		"Number of images that get pulled before removal")
	maxImages = flag.Int([]string{"-maximages"}, 0, "Maximum number of new images to process per repository (0=unlimited)")
	//nextMaxImages int
	poll = flag.Int64([]string{"p", "-poll"}, 60, "Polling interval in seconds")

	// Docker remote API related parameters
	dockerProto = flag.String([]string{"-dockerproto"}, "unix",
		"Socket protocol for Docker Remote API (\"unix\" or \"tcp\")")
	dockerAddr = flag.String([]string{"-dockeraddr"}, "/var/run/docker.sock",
		"Address of Docker remote API socket (filepath or IP:port)")

	// positional arguments: a list of repos to process, all others are ignored.
)

type RepoSet map[collector.RepoType]bool

func NewRepoSet() RepoSet {
	return make(map[collector.RepoType]bool)
}

// DoIteration runs one iteration of the main loop to get new images, extract data from them,
// and saves results.
func DoIteration(ReposToLimit RepoSet, authToken string,
	processedImages collector.ImageSet, oldMetadataSet collector.MetadataSet,
	PulledList []collector.ImageMetadataInfo) (currentMetadataSet collector.MetadataSet,
	PulledNew []collector.ImageMetadataInfo) {
	blog.Debug("DoIteration: processedImages is %v", processedImages)
	PulledNew = PulledList
	_ /*tagSlice*/, metadataSlice, currentMetadataSet := collector.GetNewImageMetadata(oldMetadataSet)

	if len(metadataSlice) == 0 {
		blog.Info("No new metadata in this iteration")
		return
	}
	blog.Info("Obtained %d new metadata items in this iteration", len(metadataSlice))
	collector.SaveImageMetadata(metadataSlice)

	// number of images processed for each repository in this iteration
	imageCount := make(map[collector.RepoType]int)
	imageToMDMap := collector.GetImageToMDMap(metadataSlice)

	// Set of repos to stop limiting according to maxImages after this iteration completes.
	StopLimiting := NewRepoSet()

	for {
		pulledImages := collector.NewImageSet()
		for _, metadata := range metadataSlice {
			if config.FilterRepos && !collector.ReposToProcess[collector.RepoType(metadata.Repo)] {
				continue
			}
			if collector.ExcludeRepo[collector.RepoType(metadata.Repo)] {
				continue
			}
			if pulledImages[collector.ImageIDType(metadata.Image)] {
				continue
			}
			repo := collector.RepoType(metadata.Repo)
			if _, ok := ReposToLimit[repo]; !ok {
				// new repo we haven't seen before; apply maxImages limit to repo
				blog.Info("Starting to apply maxImages limit to repo %s", string(repo))
				ReposToLimit[repo] = true
			}
			if ReposToLimit[repo] && *maxImages > 0 && imageCount[repo] >= *maxImages {
				blog.Info("Max image count %d reached for %s, skipping :%s",
					*maxImages, metadata.Repo, metadata.Tag)
				// stop applying the maxImages limit to repo
				StopLimiting[repo] = true
				continue
			}
			if processedImages[collector.ImageIDType(metadata.Image)] {
				continue
			}

			imageCount[collector.RepoType(metadata.Repo)]++

			// docker pull image
			collector.PullImage(metadata)
			PulledNew = append(PulledNew, metadata)
			if *removeThresh > 0 && len(PulledNew) > *removeThresh {
				collector.RemoveImages(PulledNew[0:*removeThresh], imageToMDMap)
				PulledNew = PulledNew[*removeThresh:]
			}
			pulledImages[collector.ImageIDType(metadata.Image)] = true
			if len(pulledImages) == IMAGEBATCH {
				break
			}
		}

		if len(pulledImages) == 0 {
			break
		}
		// get and save image data for all the images in pulledimages
		outMapMap := collector.GetImageAllData(pulledImages)
		collector.SaveImageAllData(outMapMap)
		for imageID := range pulledImages {
			processedImages[imageID] = true
		}
		if e := persistImageList(pulledImages); e != nil {
			blog.Error(e, "Failed to persist list of collected images")
		}
		if checkConfigUpdate(false) == true {
			break
		}
	}

	for repo := range StopLimiting {
		blog.Info("No longer enforcing maxImages limit on repo %s", repo)
		ReposToLimit[repo] = false
	}
	return
}

// getImageList reads the list of previously processed images from the imageList file.
func getImageList(processedImages collector.ImageSet) (e error) {
	f, e := os.Open(*imageList)
	if e != nil {
		blog.Warn(e, ": Error in opening", *imageList, ": perhaps a fresh start?")
		return
	}
	defer f.Close()
	r := bufio.NewReader(f)
	data, e := ioutil.ReadAll(r)
	if e != nil {
		blog.Error(e, ": Error in reading file ", *imageList)
		return
	}
	for _, str := range strings.Split(string(data), "\n") {
		if len(str) != 0 {
			blog.Debug("Previous image: %s", str)
			processedImages[collector.ImageIDType(str)] = true
		}
	}
	return
}

// persistImageList saves the set of processed images to the imageList file.
func persistImageList(collectedImages collector.ImageSet) (e error) {
	var f *os.File
	f, e = os.OpenFile(*imageList, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if e != nil {
		return
	}
	defer f.Close()
	for image := range collectedImages {
		_, e = f.WriteString(string(image) + "\n")
		if e != nil {
			return
		}
	}
	return
}

func printExampleUsage() {
	fmt.Fprintf(os.Stderr, "\n  Examples:\n")
	fmt.Fprintf(os.Stderr, "  (a) Running when compiled from source (standalone mode):\n")
	fmt.Fprintf(os.Stderr, "  \tcd <COLLECTOR_SOURCE_DIR>\n")
	fmt.Fprintf(os.Stderr, "  \tsudo COLLECTOR_DIR=$PWD $GOPATH/bin/collector index.docker.io banyanops/nginx\n\n")
	fmt.Fprintf(os.Stderr, "  (b) Running inside a Docker container: \n")
	fmt.Fprintf(os.Stderr, "  \tsudo docker run --rm \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v ~/.dockercfg:/root/.dockercfg \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v /var/run/docker.sock:/var/run/docker.sock \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v $HOME/.banyan:/banyandir \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-v <USER_SCRIPTS_DIR>:/banyancollector/data/userscripts \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\t-e BANYAN_HOST_DIR=$HOME/.banyan \\ \n")
	fmt.Fprintf(os.Stderr, "  \t\tbanyanops/collector index.docker.io banyanops/nginx\n\n")
}

// checkRepoList gets the list of repositories to process from the command line
// and from the repoList file.
func checkRepoList(initial bool) (updates bool) {
	newList := make(map[collector.RepoType]bool)

	// check repositories specified on the command line
	if len(flag.Args()) > 1 {
		for _, repo := range flag.Args()[1:] {
			newList[collector.RepoType(repo)] = true
			if initial {
				updates = true
			}
		}
	}
	// check repositories specified in the repoList file. Ignore file read errors.
	data, err := ioutil.ReadFile(*repoList)
	if err != nil {
		if initial {
			blog.Info("Repolist: " + *repoList + " not specified")
		}
	} else {
		arr := strings.Split(string(data), "\n")
		for _, line := range arr {
			// skip over comments and whitespace
			arr := strings.Split(line, "#")
			repo := arr[0]
			repotrim := strings.TrimSpace(repo)
			if repotrim != "" {
				r := collector.RepoType(repotrim)
				newList[r] = true
				if _, ok := collector.ReposToProcess[r]; !ok {
					updates = true
				}
			}
		}
	}

	//if len(collector.ReposToProcess) > 0 {
	if len(newList) > 0 {
		config.FilterRepos = true
		if updates {
			blog.Info("Limiting collection to the following repos:")
			for repo := range newList {
				blog.Info(repo)
			}
		}
	}
	collector.ReposToProcess = newList
	return
}

func setupLogging() {
	blog.AddFilter("stdout", CONSOLELOGLEVEL, blog.NewConsoleLogWriter())
	f, e := os.OpenFile(LOGFILENAME, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		blog.Exit(e, ": Error in opening log file: ", LOGFILENAME)
	}
	f.Close()
	flw := blog.NewFileLogWriter(LOGFILENAME, false)
	blog.AddFilter("file", FILELOGLEVEL, flw)
}

// copyBanyanData copies all the default scripts and binaries (e.g., bash-static, python-static, etcollector.)
// to BANYANDIR (so that it can be mounted into collector/target containers)
func copyBanyanData() {
	collector.CopyDir(config.COLLECTORDIR()+"/data/defaultscripts", collector.DefaultScriptsDir)
	//copy scripts from user specified/default directory to userScriptsDir for mounting
	collector.CopyDir(*collector.UserScriptStore, collector.UserScriptsDir)
	// * needed to copy into binDir (rather than a subdir called bin)
	collector.CopyDirTree(config.COLLECTORDIR()+"/data/bin/*", collector.BinDir)
}

func InfLoop(authToken string, processedImages collector.ImageSet, MetadataSet collector.MetadataSet,
	PulledList []collector.ImageMetadataInfo) {
	duration := time.Duration(*poll) * time.Second
	reposToLimit := NewRepoSet()

	for {
		config.BanyanUpdate("New iteration")
		MetadataSet, PulledList = DoIteration(reposToLimit, authToken, processedImages, MetadataSet, PulledList)

		blog.Info("Looping in %d seconds", *poll)
		config.BanyanUpdate("Sleeping for", strconv.FormatInt(*poll, 10), "seconds")
		time.Sleep(duration)
		checkConfigUpdate(false)
	}
}

func main() {
	doFlags()

	setupLogging()

	//verifyVolumes()

	copyBanyanData()

	// setup connection to docker daemon's unix/tcp socket
	var e error
	collector.DockerTransport, e = collector.NewDockerTransport(*dockerProto, *dockerAddr)
	if e != nil {
		blog.Exit(e, ": Error in connecting to docker remote API socket")
	}

	authToken := RegisterCollector()

	// Set output writers
	SetOutputWriters(authToken)
	SetupBanyanStatus(authToken)

	checkConfigUpdate(true)
	if collector.RegistryAPIURL == "" {
		collector.RegistryAPIURL, collector.HubAPI, collector.BasicAuth, collector.XRegistryAuth = collector.GetRegistryURL()
		blog.Info("registry API URL: %s", collector.RegistryAPIURL)
	}

	// Images we have processed already
	processedImages := collector.NewImageSet()
	e = getImageList(processedImages)
	if e != nil {
		blog.Info("Fresh start: No previously collected images were found in %s", *imageList)
	}
	blog.Debug(processedImages)

	// Image Metadata we have already seen
	MetadataSet := collector.NewMetadataSet()
	PulledList := []collector.ImageMetadataInfo{}

	// Main infinite loop.
	InfLoop(authToken, processedImages, MetadataSet, PulledList)
}
