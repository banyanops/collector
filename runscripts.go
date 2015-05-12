package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"

	blog "github.com/ccpaging/log4go"
	flag "github.com/docker/docker/pkg/mflag"
)

var (
	//userScriptsDir    = flag.String([]string{"userscriptsdir"}, BANYANDIR()+"/hosttarget/userscripts", "Directory with all user-specified scripts")
	userScriptStore   = flag.String([]string{"u", "-userscriptstore"}, COLLECTORDIR()+"/data/userscripts", "Directory with all user-specified scripts")
	userScriptsDir    = BANYANDIR() + "/hosttarget/userscripts"
	defaultScriptsDir = BANYANDIR() + "/hosttarget/defaultscripts"
	binDir            = BANYANDIR() + "/hosttarget/bin"
)

const (
	PKGEXTRACTSCRIPT = "pkgextractscript.sh"
)

func parsePkgExtractOutput(output []byte, imageID ImageIDType) (imageDataInfo []ImageDataInfo, err error) {
	type PkgInfo struct {
		Pkg, Version, Architecture string
	}

	var outInfo struct {
		DistroName string
		PkgsInfo   []PkgInfo
	}

	err = yaml.Unmarshal(output, &outInfo)
	if err != nil {
		blog.Error(err, ": Error in unmrashaling yaml")
		return
	}

	for _, pkgInfo := range outInfo.PkgsInfo {
		var imageData ImageDataInfo
		imageData.DistroName = outInfo.DistroName
		imageData.DistroID = getDistroID(outInfo.DistroName)
		imageData.Image = string(imageID)
		imageData.Pkg = pkgInfo.Pkg
		imageData.Version = pkgInfo.Version
		imageData.Architecture = pkgInfo.Architecture
		imageDataInfo = append(imageDataInfo, imageData)
	}

	return
}

func getScripts(dirPath string) (scripts []Script, err error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		blog.Warn(err, ": Error in reading contents of ", dirPath)
		return
	}

	for _, file := range files {
		file.Name()
		//figure out type of script
		var script Script
		switch {
		case strings.HasSuffix(file.Name(), ".sh"):
			blog.Debug("dirpath: " + dirPath + " after removing prefix: " + BANYANDIR() + " looks like: " + strings.TrimPrefix(dirPath, BANYANDIR()+"/hosttarget"))
			script = newBashScript(file.Name(), TARGETCONTAINERDIR+strings.TrimPrefix(dirPath, BANYANDIR()+"/hosttarget"), []string{""})
		case strings.HasSuffix(file.Name(), ".py"):
			script = newPythonScript(file.Name(), TARGETCONTAINERDIR+strings.TrimPrefix(dirPath, BANYANDIR()+"/hosttarget"), []string{""})
		default:
			blog.Warn("Unknown script file type for: " + file.Name())
			//Ignore this file...
			continue
		}
		scripts = append(scripts, script)
	}

	return
}

func getScriptsToRun() (scripts []Script) {
	// get default scripts
	defaultScripts, err := getScripts(defaultScriptsDir)
	if err != nil {
		blog.Exit(err, ": Error in getting default scripts")
	}

	// get user-specified scripts
	userScripts, err := getScripts(userScriptsDir)
	if err != nil {
		blog.Warn(err, ": Error in getting user-specified scripts")
	}

	scripts = append(defaultScripts, userScripts...)
	return
}

func runAllScripts(imageID ImageIDType) (outMap map[string]interface{}, err error) {
	//script name -> either byte array, or known types (e.g., ImageDataInfo)
	outMap = make(map[string]interface{})
	scripts := getScriptsToRun()
	for _, script := range scripts {
		//run script
		output, err := script.Run(imageID)
		if err != nil {
			blog.Error(err, ": Error in running script: ", script.Name())
			continue //continue trying to run other scripts
		}

		//analyze script output
		switch script.Name() {
		case PKGEXTRACTSCRIPT:
			imageDataInfo, err := parsePkgExtractOutput(output, imageID)
			if err != nil {
				blog.Error(err, ": Error in parsing PkgExtractOuput")
				return nil, err
			}
			outMap[script.Name()] = imageDataInfo
		default:
			//script name -> byte array
			outMap[script.Name()] = output
		}
	}

	return
}
