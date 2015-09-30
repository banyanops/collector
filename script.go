package collector

import (
	"errors"
	"strconv"

	except "github.com/banyanops/collector/except"
	blog "github.com/ccpaging/log4go"
)

// Script is the common interface to run sripts inside a container
type Script interface {
	//We expect YAML output from scripts that needs parsing of output by Banyan service
	Run(imageID ImageIDType) ([]byte, error)
	Name() string
}

// Script info for all types (e.g., bash, python, etc.)
type ScriptInfo struct {
	name         string
	dirPath      string
	params       []string
	staticBinary string
}

// Create a new bash script
func newBashScript(scriptName string, path string, params []string) Script {
	return &ScriptInfo{
		name:         scriptName,
		dirPath:      path,
		params:       params,
		staticBinary: "bash-static",
	}
}

// Create a new python script
func newPythonScript(scriptName string, path string, params []string) Script {
	return &ScriptInfo{
		name:         scriptName,
		dirPath:      path,
		params:       params,
		staticBinary: "python-static",
	}
}

// Run handles running of a script inside an image
func (sh ScriptInfo) Run(imageID ImageIDType) (b []byte, err error) {
	jsonString, err := createCmd(imageID, sh.name, sh.staticBinary, sh.dirPath)
	if err != nil {
		except.Error(err, ": Error in creating command")
		return
	}
	blog.Debug("Container spec: %s", string(jsonString))
	containerID, err := CreateContainer(jsonString)
	if err != nil {
		except.Error(err, ": Error in creating container")
		return
	}
	blog.Debug("New container ID: %s", containerID)

	defer RemoveContainer(containerID)

	jsonString, err = StartContainer(containerID)
	if err != nil {
		except.Error(err, ": Error in starting container")
		return
	}
	blog.Debug("Response from StartContainer: %s", string(jsonString))
	statusCode, err := WaitContainer(containerID)
	if err != nil {
		except.Error(err, ": Error in waiting for container to stop")
		return
	}
	if statusCode != 0 {
		err = errors.New("Bash script exit status: " + strconv.Itoa(statusCode))
		return
	}
	b, err = LogsContainer(containerID)
	if err != nil {
		except.Error(err, ":Error in extracting output from container")
		return
	}
	/*
		_, err = removeContainer(containerID)
		if err != nil {
			except.Error(err, ":Error in removing container for image", containerID)
			return
		}
	*/
	return
}

// Name gives the name of the script
func (sh ScriptInfo) Name() string {
	return sh.name
}
