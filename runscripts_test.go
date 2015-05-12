package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

const (
	DOCKERPROTO = "unix"
	DOCKERADDR  = "/var/run/docker.sock"
)

func TestCreateCmd(t *testing.T) {
	os.Setenv("HOSTNAME", "")
	jsonString, err := createCmd(ImageIDType("imageID"), "scriptName", "staticBinary", "dirPath")
	if err != nil {
		t.Fatal(err)
	}
	dst := bytes.NewBuffer([]byte{})
	json.Indent(dst, jsonString, "", "    ")
	fmt.Println(string(dst.Bytes()))
}

func TestBashScriptRun(t *testing.T) {
	var e error
	DockerTransport, e = NewDockerTransport(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	registryspec = "index.docker.io"
	registryAPIURL, hubAPI, XRegistryAuth = getRegistryURL()
	metadata := ImageMetadataInfo{
		Repo: "ubuntu",
		Tag:  "latest",
	}
	fmt.Println("TestPullImage %v", metadata)
	PullImage(metadata)

	os.Setenv("BANYAN_HOST_DIR", "/tmp/banyandir")
	createDirIfNotExist("/tmp/banyandir/hosttarget/bin")
	createDirIfNotExist("/tmp/banyandir/hosttarget/defaultscripts")
	copyDirTree(os.Getenv("PWD")+"/data/bin/*", "/tmp/banyandir/hosttarget/bin")
	copyDir(os.Getenv("PWD")+"/data/defaultscripts", "/tmp/banyandir/hosttarget/defaultscripts")
	bs := newBashScript("pkgextractscript.sh", "/banyancollector/defaultscripts", []string{})
	b, err := bs.Run(ImageIDType("ubuntu"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Run returned", string(b))
}

func TestPostDockerAPI(t *testing.T) {
	tr, e := NewDockerTransport(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	apipath := "/containers/create"
	jsonString := []byte(`{ "Hostname": "", "User": "0", "AttachStdin": false, "AttachStdout": true, "AttachStderr": true, "Tty": false, "Env": null, "Cmd": [ "-c", "PATH=/banyancollector:$PATH /banyancollector/pkgextractscript.sh" ], "Entrypoint": [ "/banyancollector/bash-static" ], "Image": "ubuntu", "WorkingDir": "", "HostConfig": { "Binds": [ "/home/yoshiotu/gospace/src/bitbucket.org/banyanops/collector/docker/data:/banyancollector:ro" ], "Links": null, "Privileged": false, "VolumesFrom": null } }`)
	resp, err := doDockerAPI(tr, "POST", apipath, jsonString, "")
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Id       string
		Warnings string
	}
	e = json.Unmarshal(resp, &msg)
	if e != nil {
		fmt.Println(e, "jsonunmarshal failed for string", string(resp))
		return
	}
	fmt.Printf("Got ID %s Warnings %s\n", msg.Id, msg.Warnings)
}
