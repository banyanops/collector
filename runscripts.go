package collector

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"

	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
	blog "github.com/ccpaging/log4go"
	flag "github.com/spf13/pflag"
)

var (
	//userScriptsDir    = flag.String([]string{"userscriptsdir"}, config.BANYANDIR()+"/hosttarget/userscripts", "Directory with all user-specified scripts")
	UserScriptStore   = flag.StringP("userscriptstore", "u", config.COLLECTORDIR()+"/data/userscripts", "Directory with all user-specified scripts")
	UserScriptsDir    = config.BANYANDIR() + "/hosttarget/userscripts"
	DefaultScriptsDir = config.BANYANDIR() + "/hosttarget/defaultscripts"
	BinDir            = config.BANYANDIR() + "/hosttarget/bin"
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
		except.Error(err, ": Error in unmrashaling yaml")
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
	// if no package info was found, return a single entry with empty string in the package info fields.
	if len(outInfo.PkgsInfo) == 0 {
		var imageData ImageDataInfo
		imageData.DistroName = outInfo.DistroName
		imageData.DistroID = getDistroID(outInfo.DistroName)
		imageData.Image = string(imageID)
		imageData.Pkg = ""
		imageData.Version = ""
		imageData.Architecture = ""
		imageDataInfo = append(imageDataInfo, imageData)
	}

	return
}

func getScripts(dirPath string) (scripts []Script, err error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		except.Warn(err, ": Error in reading contents of ", dirPath)
		return
	}

	for _, file := range files {
		file.Name()
		//figure out type of script
		var script Script
		switch {
		case strings.HasSuffix(file.Name(), ".sh"):
			blog.Debug("dirpath: " + dirPath + " after removing prefix: " + config.BANYANDIR() + " looks like: " + strings.TrimPrefix(dirPath, config.BANYANDIR()+"/hosttarget"))
			script = newBashScript(file.Name(), TARGETCONTAINERDIR+strings.TrimPrefix(dirPath, config.BANYANDIR()+"/hosttarget"), []string{""})
		case strings.HasSuffix(file.Name(), ".py"):
			script = newPythonScript(file.Name(), TARGETCONTAINERDIR+strings.TrimPrefix(dirPath, config.BANYANDIR()+"/hosttarget"), []string{""})
		default:
			except.Warn("Unknown script file type for: " + file.Name())
			//Ignore this file...
			continue
		}
		scripts = append(scripts, script)
	}

	return
}

func getScriptsToRun() (scripts []Script) {
	// get default scripts
	defaultScripts, err := getScripts(DefaultScriptsDir)
	if err != nil {
		except.Fail(err, ": Error in getting default scripts")
	}

	// get user-specified scripts
	userScripts, err := getScripts(UserScriptsDir)
	if err != nil {
		except.Warn(err, ": Error in getting user-specified scripts")
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
			except.Error(err, ": Error in running script: ", script.Name())
			continue //continue trying to run other scripts
		}

		//analyze script output
		switch script.Name() {
		case PKGEXTRACTSCRIPT:
			imageDataInfo, err := parsePkgExtractOutput(output, imageID)
			if err != nil {
				except.Error(err, ": Error in parsing PkgExtractOuput")
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
