// copied from :
// https://github.com/banzaicloud/prometheus-jmx-exporter-operator/blob/master/pkg/stub/copy.go
// https://github.com/banzaicloud/prometheus-jmx-exporter-operator/blob/master/pkg/stub/exec.go

package controllers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"net"
	"os"
	"path"
	"strings"
)

var (
	kubeClient      *kubernetes.Clientset
	inClusterConfig *rest.Config
)

func dontPanicOnTest(err error) {
	if flag.Lookup("test.v") != nil {
		panic(err)
	} else {
		fmt.Println(err)
	}
}

func init() {
	// Work around https://github.com/kubernetes/kubernetes/issues/40973
	// See https://github.com/coreos/etcd-operator/issues/731#issuecomment-283804819
	if len(os.Getenv("KUBERNETES_SERVICE_HOST")) == 0 {
		addrs, err := net.LookupHost("kubernetes.default.svc")
		if err != nil {
			dontPanicOnTest(err)
			return
		}
		os.Setenv("KUBERNETES_SERVICE_HOST", addrs[0])
	}
	if len(os.Getenv("KUBERNETES_SERVICE_PORT")) == 0 {
		os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	}

	var err error
	inClusterConfig, err = rest.InClusterConfig()

	if err != nil {
		dontPanicOnTest(err)
		return
	}

	kubeClient = kubernetes.NewForConfigOrDie(inClusterConfig)
}

// ExecCommand executes the given command inside the specified container remotely
func ExecCommand(namespace, podName string, stdinReader io.Reader, container *v1.Container, command ...string) (string, error) {

	execReq := kubeClient.CoreV1().RESTClient().Post()
	execReq = execReq.Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")

	execReq.VersionedParams(&v1.PodExecOptions{
		Container: container.Name,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
		Stdin:     stdinReader != nil,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(inClusterConfig, "POST", execReq.URL())

	if err != nil {
		logrus.Errorf("Creating remote command executor failed: %v", err)
		return "", err
	}

	stdOut := bytes.Buffer{}
	stdErr := bytes.Buffer{}

	logrus.Debugf("Executing command '%v' in namespace='%s', pod='%s', container='%s'", command, namespace, podName, container.Name)
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: bufio.NewWriter(&stdOut),
		Stderr: bufio.NewWriter(&stdErr),
		Stdin:  stdinReader,
		Tty:    false,
	})

	logrus.Debugf("Command stderr: %s", stdErr.String())
	logrus.Debugf("Command stdout: %s", stdOut.String())

	if err != nil {
		logrus.Infof("Executing command failed with: %v", err)

		return "", err
	}

	logrus.Debug("Command succeeded.")
	if stdErr.Len() > 0 {
		return "", fmt.Errorf("stderr: %v", stdErr.String())
	}

	return stdOut.String(), nil

}

// copyToPod uploads the content of srcDir to destDir on given container of the pod identified by podName
// in namespace
func CopyToPod(namespace, podName string, container *v1.Container, srcDir, destDir string) error {
	logrus.Infof("Copying the content of '%s' directory to '%s/%s/%s:%s'", srcDir, namespace, podName, container.Name, destDir)

	ok, err := checkSourceDir(srcDir)
	if err != nil {
		return err
	}

	if !ok {
		logrus.Warnf("Source directory '%s' is empty. There is nothing to copy.", srcDir)
		return nil
	}

	if destDir != "/" && strings.HasSuffix(destDir, "/") {
		destDir = strings.TrimSuffix(destDir, "/")
	}

	err = createDestDirIfNotExists(namespace, podName, container, destDir)
	if err != nil {
		logrus.Errorf("Creating destination directory failed: %v", err)
		return err
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()

		err := makeTar(srcDir, ".", writer)
		if err != nil {
			logrus.Errorf("Making tar file of '%s' failed: %v", srcDir, err)
		}
	}()

	_, err = ExecCommand(namespace, podName, reader, container, "tar", "xfm", "-", "-C", destDir)

	logrus.Infof("Copying the content of '%s' directory to '%s/%s/%s:%s' finished", srcDir, namespace, podName, container.Name, destDir)
	return err
}

// checkSourceDir verifies if path exists and it's not an empty directory
func checkSourceDir(path string) (bool, error) {
	f, err := os.Open(path)

	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)

	if err == io.EOF {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// createDestDirIfNotExists creates the directory dirPath if not exists
// on the target pod container
func createDestDirIfNotExists(namespace, podName string, container *v1.Container, dirPath string) error {
	logrus.Infof("Creating '%s/%s/%s:%s' if not exists.", namespace, podName, container.Name, dirPath)

	_, err := ExecCommand(namespace, podName, nil, container,
		"mkdir", "-p", dirPath)

	return err
}

// makeTar tars the files and subdirectories of srcDir into tarDestDir (root directory within the tar file)
// than writes the created tar file to writer
func makeTar(srcDir, tarDestDir string, writer io.Writer) error {
	srcDirPath := path.Clean(srcDir)
	tarDestDirPath := path.Clean(tarDestDir)

	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	return makeTarRec(srcDirPath, tarDestDirPath, tarWriter)
}

// makeTarRec tars recursively the content of srcDirPath
func makeTarRec(srcPath, tarDestPath string, writer *tar.Writer) error {
	stat, err := os.Lstat(srcPath)
	if err != nil {
		return err
	}

	if stat.IsDir() {
		files, err := ioutil.ReadDir(srcPath)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			// empty dir
			hdr, err := tar.FileInfoHeader(stat, srcPath)
			if err != nil {
				return err
			}

			hdr.Name = tarDestPath
			if err := writer.WriteHeader(hdr); err != nil {
				return err
			}
		}

		for _, f := range files {
			if err := makeTarRec(path.Join(srcPath, f.Name()), path.Join(tarDestPath, f.Name()), writer); err != nil {
				return err
			}
		}
	} else if stat.Mode()&os.ModeSymlink != 0 {
		//case soft link
		hdr, _ := tar.FileInfoHeader(stat, srcPath)
		target, err := os.Readlink(srcPath)
		if err != nil {
			return err
		}

		hdr.Linkname = target
		hdr.Name = tarDestPath
		if err := writer.WriteHeader(hdr); err != nil {
			return err
		}
	} else {
		//case regular file or other file type like pipe
		hdr, err := tar.FileInfoHeader(stat, srcPath)
		if err != nil {
			return err
		}
		hdr.Name = tarDestPath

		if err := writer.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(writer, f); err != nil {
			return err
		}
		return f.Close()
	}

	return nil
}
