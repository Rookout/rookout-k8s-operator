// Inspired from :
// https://github.com/banzaicloud/prometheus-jmx-exporter-operator/blob/master/pkg/stub/copy.go
// https://github.com/banzaicloud/prometheus-jmx-exporter-operator/blob/master/pkg/stub/exec.go

package controllers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"errors"
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
	"strconv"
	"strings"
)

var (
	kubeClient      *kubernetes.Clientset
	inClusterConfig *rest.Config
)

var SHELLS = []string{"sh", "ash"}

type PodUtils struct {
	namespace   string
	podName     string
	shell       string
	stdinReader io.Reader
	container   *v1.Container
}

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

func NewPodUtils(namespace, podName string, stdinReader io.Reader, container *v1.Container) (PodUtils, error) {
	p := PodUtils{
		namespace:   namespace,
		podName:     podName,
		stdinReader: stdinReader,
		container:   container,
	}

	shell, err := p.detectShell()

	if err != nil {
		return p, err
	}

	p.shell = shell
	return p, nil
}

func (p PodUtils) ExecCommand(useShell bool, command ...string) (string, error) {
	var wrappedCommand []string

	if useShell {
		wrappedCommand = append(wrappedCommand, []string{p.shell, "-c"}...)
	}

	wrappedCommand = append(wrappedCommand, command...)

	logrus.Infof("executing command: %s", wrappedCommand)

	execReq := kubeClient.CoreV1().RESTClient().Post()
	execReq = execReq.Resource("pods").Name(p.podName).Namespace(p.namespace).SubResource("exec")

	execReq.VersionedParams(&v1.PodExecOptions{
		Container: p.container.Name,
		Command:   wrappedCommand,
		Stdout:    true,
		Stderr:    true,
		Stdin:     p.stdinReader != nil,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(inClusterConfig, "POST", execReq.URL())

	if err != nil {
		return "", err
	}

	stdOut := bytes.Buffer{}
	stdErr := bytes.Buffer{}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: bufio.NewWriter(&stdOut),
		Stderr: bufio.NewWriter(&stdErr),
		Stdin:  p.stdinReader,
		Tty:    false,
	})

	if err != nil {
		return "", err
	}

	if stdErr.Len() > 0 {
		return "", fmt.Errorf("stderr: %v", stdErr.String())
	}

	return stdOut.String(), nil

}

func (p PodUtils) CopyToPod(srcDir, destDir string) error {
	logrus.Infof("Copying the content of '%s' directory to '%s/%s/%s:%s'", srcDir, p.namespace, p.podName, p.container.Name, destDir)

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

	err = p.createDestDirIfNotExists(destDir)
	if err != nil {
		logrus.Errorf("Creating destination directory failed: %v", err)
		return err
	}

	reader, writer := io.Pipe()
	p.stdinReader = reader
	go func() {
		defer writer.Close()

		err := makeTar(srcDir, ".", writer)
		if err != nil {
			logrus.Errorf("Making tar file of '%s' failed: %v", srcDir, err)
		}
	}()

	_, err = p.ExecCommand(false, "tar", "xfm", "-", "-C", destDir)

	logrus.Infof("Copying the content of '%s' directory to '%s/%s/%s:%s' finished", srcDir, p.namespace, p.podName, p.container.Name, destDir)
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
func (p PodUtils) createDestDirIfNotExists(dirPath string) error {
	logrus.Infof("Creating '%s/%s/%s:%s' if not exists.", p.namespace, p.podName, p.container.Name, dirPath)

	_, err := p.ExecCommand(false, "mkdir", "-p", dirPath)

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

func (p PodUtils) detectShell() (string, error) {

	for _, shell := range SHELLS {
		_, err := p.ExecCommand(false, shell, "-c", "ls")
		if err != nil {
			continue
		}

		return shell, nil
	}

	return "", errors.New("no shell detected")
}

func extractMatchedPids(stdout string, matchString string) ([]int, error) {
	var javaProcIds []int

	procs := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, proc := range procs {
		if !strings.Contains(proc, matchString) {
			continue
		}

		pidStr := ""
		idAndClassName := strings.Split(proc, " ")
		for _, element := range idAndClassName {
			if element != "" {
				pidStr = element
				break
			}
		}

		if pidStr == "" {
			return nil, errors.New("pid not found")
		}

		if pidStr == "" {
			continue
		}

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			return nil, err
		}

		javaProcIds = append(javaProcIds, pid)
	}

	return javaProcIds, nil
}

func (p PodUtils) QueryMatchedProcesses(matchStr string) ([]int, error) {
	stdout, err := p.ExecCommand(true, PS_CMD)

	if err != nil {
		return nil, err
	} else {

		ProcessIds, extractErr := extractMatchedPids(stdout, matchStr)
		if extractErr != nil {
			return nil, err
		}

		return ProcessIds, nil
	}
}
