package main

// Collector is a program that extracts static information from container images stored in a Docker registry.

import (
	"bufio"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	collector "github.com/banyanops/collector"
	auth "github.com/banyanops/collector/auth"
	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
	fsutil "github.com/banyanops/collector/fsutil"
	blog "github.com/ccpaging/log4go"
	flag "github.com/spf13/pflag"
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
	fileLog     = flag.Bool("filelog", false, "Log output to "+LOGFILENAME)
	imageList   = flag.String("imagelist", config.BANYANDIR()+"/hostcollector/imagelist",
		"List of previously collected images (file)")
	repoList = flag.StringP("repolist", "r", config.BANYANDIR()+"/hostcollector/repolist",
		"File containing list of repos to process")

	// Configuration parameters for speed/efficiency
	removeThresh = flag.Int("removethresh", 5,
		"Number of images that get pulled before removal")
	maxImages = flag.Int("maximages", 0, "Maximum number of new images to process per repository (0=unlimited)")
	//nextMaxImages int
	poll = flag.Int64P("poll", "p", 60, "Polling interval in seconds")

	// Docker remote API related parameters
	dockerProto = flag.String("dockerproto", "unix",
		"Socket protocol for Docker Remote API (\"unix\" or \"tcp\")")
	dockerAddr = flag.String("dockeraddr", "/var/run/docker.sock",
		"Address of Docker remote API socket (filepath or IP:port)")

	// Docker Registry rate limiting
	maxRequests  = flag.Int("maxreq", 0, "max # of requests to registry in time period (0 for no limit)")
	maxRequests2 = flag.Int("maxreq2", 0, "max # of requests to registry in time period 2 (0 for no limit)")
	timePeriod   = flag.Duration("timeper", 10*time.Minute, "registry request rate limiting time period")
	timePeriod2  = flag.Duration("timeper2", 24*time.Hour, "registry request rate limiting time period 2")

	// positional arguments: a list of repos to process, all others are ignored.
)

func init() {
	toHide := flag.Lookup("imagelist")
	if toHide != nil {
		toHide.Hidden = true
	}
}

type RepoSet map[collector.RepoType]bool

func NewRepoSet() RepoSet {
	return make(map[collector.RepoType]bool)
}

// updateRepoTagImageID removes obsolete metadata when a pull determines that
// a new image ID is associated with a repo:tag.
func updateRepoTagImageID(metadata *collector.ImageMetadataInfo, oldMetadataSet collector.MetadataSet) {
	matches := oldMetadataSet.SameRepoTag(*metadata)
	obsolete := []collector.ImageMetadataInfo{}
	for _, m := range matches {
		if metadata.Image != m.Image {
			obsolete = append(obsolete, m)
		}
	}
	if len(obsolete) > 0 {
		collector.RemoveObsoleteMetadata(obsolete)
	}
}

// DoIteration runs one iteration of the main loop to get new images, extract data from them,
// and save results.
func DoIteration(ReposToLimit RepoSet, tokenSync *auth.TokenSyncInfo,
	processedImages collector.ImageSet, oldMetadataSet collector.MetadataSet,
	PulledList []collector.ImageMetadataInfo) (currentMetadataSet collector.MetadataSet,
	PulledNew []collector.ImageMetadataInfo) {

	blog.Debug("DoIteration: processedImages is %v", processedImages)
	PulledNew = PulledList
	metadataSlice, currentMetadataSet := collector.GetNewImageMetadata(oldMetadataSet)

	if len(metadataSlice) == 0 {
		blog.Info("No new metadata in this iteration")
		return
	}
	blog.Info("Obtained %d new metadata items in this iteration", len(metadataSlice))
	collector.SaveImageMetadata(metadataSlice)

	// number of images processed for each repository in this iteration
	imageCount := make(map[collector.RepoType]int)

	// Set of repos to stop limiting according to maxImages after this iteration completes.
	StopLimiting := NewRepoSet()

	// processed metadata
	processedMetadata := collector.NewMetadataSet()

	for {
		pulledImages := collector.NewImageSet()
		pulledImagesManifestHash := collector.NewImageSet()
		pullErrorMetadata := collector.NewMetadataSet()
		for index, _ := range metadataSlice {
			metadata := &metadataSlice[index]
			processedMetadata.Insert(*metadata)
			if config.FilterRepos && !collector.ReposToProcess[collector.RepoType(metadata.Repo)] {
				continue
			}
			// TODO: need to filter out images from ExcludedRepo also when collecting from local Docker host?
			if collector.ExcludeRepo[collector.RepoType(metadata.Repo)] {
				continue
			}
			if len(metadata.Image) > 0 && pulledImages.Exists(collector.ImageIDType(metadata.Image)) {
				continue
			}
			if len(metadata.ManifestHash) > 0 &&
				pulledImagesManifestHash.Exists(collector.ImageIDType(metadata.ManifestHash)) {
				continue
			}
			// TODO: need to consider maxImages limit also when collecting from local Docker host?
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
			if len(metadata.Image) > 0 && processedImages.Exists(collector.ImageIDType(metadata.Image)) {
				continue
			}
			if len(metadata.ManifestHash) > 0 && processedImages.Exists(collector.ImageIDType(metadata.ManifestHash)) {
				continue
			}

			imageCount[collector.RepoType(metadata.Repo)]++

			ImageLenBeforePull := len(metadata.Image)

			// docker pull image
			if !collector.LocalHost {
				err := collector.PullImage(metadata)
				if err != nil {
					// docker pull failed for some reason, possibly a transient failure.
					// So we remove this metadata element from the current and processed sets,
					// and move on to process any remaining metadata elements.
					// In the next iteration, metadata
					// lookup may rediscover this deleted metadata element
					// and treat it as new, thus ensuring that the image pull will be retried.
					// TODO: If the registry is corrupted, this can lead to an infinite
					// loop in which the same image pull keeps getting tried and consistently fails.
					currentMetadataSet.Delete(*metadata)
					processedMetadata.Delete(*metadata)
					// remember this pull error in order to demote this metadata to the end of the slice.
					pullErrorMetadata.Insert(*metadata)
					err = collector.RemoveDanglingImages()
					if err != nil {
						except.Error(err, ": RemoveDanglingImages")
					}
					continue
				}
				updateRepoTagImageID(metadata, oldMetadataSet)
				processedMetadata.Replace(*metadata)
				if ImageLenBeforePull == 0 && len(metadata.Image) > 0 {
					// Docker daemon computed the image ID for us, so now we can record this entry.
					collector.SaveImageMetadata([]collector.ImageMetadataInfo{*metadata})
				}
			}
			PulledNew = append(PulledNew, *metadata)
			excess := len(PulledNew) - *removeThresh
			if !collector.LocalHost && *removeThresh > 0 && excess > 0 {
				config.BanyanUpdate("Removing " + strconv.Itoa(excess) + " pulled images")
				collector.RemoveImages(PulledNew[0:excess])
				PulledNew = PulledNew[excess:]
			}
			blog.Info("Added image %s to pulledImages", metadata.Image)
			pulledImages.Insert(collector.ImageIDType(metadata.Image))
			pulledImagesManifestHash.Insert(collector.ImageIDType(metadata.ManifestHash))
			if len(pulledImages) == IMAGEBATCH {
				break
			}
		}

		if len(pulledImages) == 0 {
			blog.Info("No pulled images left to process in this iteration")
			config.BanyanUpdate("No pulled images left to process in this iteration")
			break
		}

		// reorder metadataSlice by moving images that couldn't be pulled to the end of the list
		newMDSlice := []collector.ImageMetadataInfo{}
		for _, metadata := range metadataSlice {
			if !pullErrorMetadata.Exists(metadata) {
				newMDSlice = append(newMDSlice, metadata)
			}
		}
		for metadata := range pullErrorMetadata {
			newMDSlice = append(newMDSlice, metadata)
		}
		metadataSlice = newMDSlice

		// get and save image data for all the images in pulledimages
		outMapMap := collector.GetImageAllData(pulledImages)
		collector.SaveImageAllData(outMapMap)
		for imageID := range pulledImages {
			processedImages.Insert(imageID)
		}
		for manifestHash := range pulledImagesManifestHash {
			processedImages.Insert(manifestHash)
		}
		if e := persistImageList(pulledImages); e != nil {
			except.Error(e, "Failed to persist list of collected images")
		}
		if e := persistImageManifestHashList(pulledImagesManifestHash); e != nil {
			except.Error(e, "Failed to persist list of collected image manifest hashes")
		}
		if checkConfigUpdate(false) == true {
			// Config changed, and possibly did so before all current metadata was processed.
			// Thus, remember only the metadata that has already been processed, and forget
			// metadata that has not been processed yet.
			// That way, the next time DoIteration() is entered, the metadata lookup
			// will correctly schedule the forgotten metadata for processing, along with
			// any new metadata.
			currentMetadataSet = processedMetadata
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
		except.Warn(e, ": Error in opening", *imageList, ": perhaps a fresh start?")
		return
	}
	defer f.Close()
	r := bufio.NewReader(f)
	data, e := ioutil.ReadAll(r)
	if e != nil {
		except.Error(e, ": Error in reading file ", *imageList)
		return
	}
	for _, str := range strings.Split(string(data), "\n") {
		if len(str) != 0 {
			blog.Debug("Previous image: %s", str)
			processedImages.Insert(collector.ImageIDType(str))
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

// getImageManifestHashList reads the list of previously processed images (manifest hash) from the imageList_ManifestHash file.
func getImageManifestHashList(processedImagesManifestHash collector.ImageSet) (e error) {
	filename := *imageList + "_ManifestHash"
	f, e := os.Open(filename)
	if e != nil {
		except.Warn(e, ": Error in opening", filename, ": perhaps a fresh start?")
		return
	}
	defer f.Close()
	r := bufio.NewReader(f)
	data, e := ioutil.ReadAll(r)
	if e != nil {
		except.Error(e, ": Error in reading file ", filename)
		return
	}
	for _, str := range strings.Split(string(data), "\n") {
		if len(str) != 0 {
			blog.Debug("Previous image: %s", str)
			processedImagesManifestHash.Insert(collector.ImageIDType(str))
		}
	}
	return
}

// persistImageManifestHashList saves the set of processed image manifest hashes to the imageList_ManifestHash file.
func persistImageManifestHashList(collectedImagesManifestHash collector.ImageSet) (e error) {
	var f *os.File
	filename := *imageList + "_ManifestHash"
	f, e = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if e != nil {
		return
	}
	defer f.Close()
	for manifestHash := range collectedImagesManifestHash {
		_, e = f.WriteString(string(manifestHash) + "\n")
		if e != nil {
			return
		}
	}
	return
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

	if len(newList) == 0 {
		collector.ReposToProcess = newList
		return
	}
	collector.ReposToProcess = newList
	if searchTerm := collector.NeedRegistrySearch(); searchTerm != "" {
		config.FilterRepos = false
	} else {
		config.FilterRepos = true
	}
	if updates {
		blog.Info("Limiting collection to the following repos:")
		for repo := range newList {
			blog.Info(repo)
		}
	}
	return
}

func setupLogging() {
	consoleLog := blog.NewConsoleLogWriter()
	consoleLog = consoleLog.SetColor(true)
	blog.AddFilter("stdout", CONSOLELOGLEVEL, consoleLog)
	if *fileLog == true {
		f, e := os.OpenFile(LOGFILENAME, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if e != nil {
			except.Fail(e, ": Error in opening log file: ", LOGFILENAME)
		}
		f.Close()
		flw := blog.NewFileLogWriter(LOGFILENAME)
		blog.AddFilter("file", FILELOGLEVEL, flw)
	}
}

// copyBanyanData copies all the default scripts and binaries (e.g., bash-static, python-static, etc.)
// to BANYANDIR (so that it can be mounted into collector/target containers)
func copyBanyanData() {
	fsutil.CopyDir(config.COLLECTORDIR()+"/data/defaultscripts", collector.DefaultScriptsDir)
	//copy scripts from user specified/default directory to userScriptsDir for mounting
	fsutil.CopyDir(*collector.UserScriptStore, collector.UserScriptsDir)
	// * needed to copy into binDir (rather than a subdir called bin)
	fsutil.CopyDirTree(config.COLLECTORDIR()+"/data/bin/*", collector.BinDir)
}

func InfLoop(tokenSync *auth.TokenSyncInfo, processedImages collector.ImageSet) {
	duration := time.Duration(*poll) * time.Second
	reposToLimit := NewRepoSet()

	// Image Metadata we have already seen
	metadataSet := collector.NewMetadataSet()
	initMetadataSet(tokenSync, metadataSet)
	pulledList := []collector.ImageMetadataInfo{}

	for {
		config.BanyanUpdate("New iteration")
		metadataSet, pulledList = DoIteration(reposToLimit, tokenSync, processedImages, metadataSet, pulledList)

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
	collector.DockerClient, e = collector.NewDockerClient(*dockerProto, *dockerAddr)
	if e != nil {
		except.Fail(e, ": Error in connecting to docker remote API socket")
	}

	var tokenSync auth.TokenSyncInfo
	tokenSync.SetApplication("collector")
	RegisterCollector(&tokenSync)

	// Set output writers
	SetOutputWriters(&tokenSync)
	SetupBanyanStatus(&tokenSync)

	checkConfigUpdate(true)
	if collector.LocalHost == false && collector.RegistryAPIURL == "" {
		collector.RegistryAPIURL, collector.HubAPI, collector.BasicAuth, collector.XRegistryAuth = collector.GetRegistryURL()
		blog.Info("registry API URL: %s", collector.RegistryAPIURL)
	}

	// Log the docker version
	major, minor, revision, e := collector.DockerVersion()
	if e != nil {
		except.Error(e, ": Could not identify Docker version")
	} else {
		blog.Info("Docker version %d.%d.%d", major, minor, revision)
		config.BanyanUpdate("Docker version", strconv.Itoa(major)+"."+strconv.Itoa(minor)+"."+strconv.Itoa(revision))
	}

	// Images we have processed already
	processedImages := collector.NewImageSet()
	e = getImageList(processedImages)
	if e != nil {
		blog.Info("Fresh start: No previously collected images were found in %s", *imageList)
	}
	_ = getImageManifestHashList(processedImages)
	blog.Debug(processedImages)

	// Main infinite loop.
	InfLoop(&tokenSync, processedImages)
}
