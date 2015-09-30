//Package fsutil provides filesystem utilities."
package fsutil

import (
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	except "github.com/banyanops/collector/except"
)

func DirExists(dir string) (bool, error) {
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// dir doesn't exist
			return false, nil
		} else {
			// other error
			except.Error(err, ": Error while looking up directory: ", dir)
			return false, err
		}
	}

	if !fi.IsDir() {
		err = errors.New("A file already exists with this name: " + dir)
		return false, err
	}

	return true, nil
}

func CreateDirIfNotExist(dir string) (err error) {
	exists, err := DirExists(dir)
	if err != nil {
		except.Error(err, ": Error while querying dir: ", dir)
		return err
	}
	if !exists {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			except.Error(err, ": Error in creating dir: ", dir)
			return err
		}
	}

	return nil
}

func copy(src, dest string) {
	// Read all content of src to data
	data, err := ioutil.ReadFile(src)
	if err != nil {
		except.Error(err, ": Error in reading from file: ", src)
		return
	}
	// Write data to dest
	err = ioutil.WriteFile(dest, data, 0755)
	if err != nil {
		except.Error(err, ": Error in writing to file: ", dest)
		return
	}
}

// copyDir copies all files from srcDir to destDir
func CopyDir(srcDir, destDir string) {
	existsSrc, err1 := DirExists(srcDir)
	existsDest, err2 := DirExists(destDir)
	if err1 != nil || err2 != nil {
		//detailed error handling inside DirExists
		return
	}
	if !existsSrc || !existsDest {
		except.Error("Src/Dest directories don't exist: srcdir: " + srcDir + " destdir: " + destDir)
		return
	}
	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		except.Error(err, ": Error in reading contents of ", srcDir)
		return
	}

	for _, file := range files {
		copy(srcDir+"/"+file.Name(), destDir+"/"+file.Name())
	}

}

// CopyDirTree copies all files from srcDir to destDir
func CopyDirTree(srcDir, destDir string) {
	srcs, err := filepath.Glob(srcDir)
	if err != nil {
		except.Fail(err, ": Error in generating matches for", srcDir)
	}
	args := []string{"-rp"}
	dirs := append(args, append(srcs, destDir)...)
	cpCmd := exec.Command("cp", dirs...)
	err = cpCmd.Run()
	if err != nil {
		except.Fail(err, ": Error in copying", srcDir, " to ", destDir)
	}
}
