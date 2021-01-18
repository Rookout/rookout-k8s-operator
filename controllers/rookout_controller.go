/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rookoutv1alpha1 "github.com/rookout/rookout-k8s-operator/api/v1alpha1"
)

// RookoutReconciler reconciles a Rookout object
type RookoutReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

const (
	POD_NAME          = "java-test"
	SRC_DIR           = "/var/rookout"
	DST_DIR           = "/rookout"
	PS_CMD            = "ps"
	JAVA_PROC_MATCHER = "java -jar"
)

// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/finalizers,verbs=update

// Annotation for generating RBAC role to Watch Pods
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;watch;list
// +kubebuilder:rbac:groups="",resources="pods/exec",verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Rookout object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *RookoutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := v1.Pod{}
	err := r.Client.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !strings.Contains(pod.Name, POD_NAME) {
		return ctrl.Result{}, nil
	}

	if len(pod.Status.ContainerStatuses) > 0 {
		logrus.Infof("namespace: %s, name: %s, is ready : %v, status len: %v", req.Namespace, req.Name, pod.Status.ContainerStatuses[0].Ready, len(pod.Status.ContainerStatuses))
		if !pod.Status.ContainerStatuses[0].Ready {
			return ctrl.Result{}, nil
		}
	}

	for _, container := range pod.Spec.Containers {
		shell, shellErr := DetectShell(pod.Namespace, pod.Name, &container)
		if shellErr != nil {
			logrus.Errorf("Failed to detect shell on container %v", container.Name)
			continue
		}

		javaPids, pidErr := queryJavaProcesses(shell, &pod, &container)
		if pidErr != nil {
			logrus.WithField("err", pidErr).Errorf("Failed to retrieve java processes from container %v", container.Name)
			continue
		}

		if len(javaPids) == 0 {
			continue
		}

		logrus.Infof("container: %s, java processes: %v", container.Name, javaPids)

		copyErr := CopyToPod(pod.Namespace, pod.Name, &container, SRC_DIR, DST_DIR)
		if copyErr != nil {
			logrus.WithField("err", copyErr).Errorf("Failed to copy rook loader to container %v", container.Name)
			continue
		}

		// disable reflection warning
		// ref : https://nipafx.dev/five-command-line-options-hack-java-module-system/
		javaFlags := "--add-opens java.base/java.net=ALL-UNNAMED"
		token := "fba5d2d413de317d77110867968ecc413bc13e65a7c75a32f6002adb2d7aebee"

		for _, pid := range javaPids {
			loadCmd := fmt.Sprintf("ROOKOUT_TOKEN=%s ROOKOUT_TARGET_PID=%d java %s -jar %s/rook.jar", token, pid, javaFlags, DST_DIR)
			stdout, execErr := ExecCommand(pod.Namespace, pod.Name, nil, &container, shell, "-c", loadCmd)
			if execErr != nil {
				logrus.WithField("err", execErr).Errorf("failed to inject rook to pid %d", pid)
				continue
			}
			logrus.Info(stdout)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RookoutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}).
		For(&rookoutv1alpha1.Rookout{}).
		Complete(r)
}

func extractMatchedPids(stdout string, matchString string) ([]int, error) {
	var javaProcIds []int

	procs := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, proc := range procs {
		if !strings.Contains(proc, matchString) {
			continue
		}

		idAndClassName := strings.Split(proc, " ")
		trimmedPid := strings.Trim(strings.TrimSpace(idAndClassName[4]), "\t")

		if trimmedPid == "" {
			continue
		}

		pid, err := strconv.Atoi(trimmedPid)
		if err != nil {
			return nil, err
		}

		javaProcIds = append(javaProcIds, pid)
	}

	return javaProcIds, nil
}

// queryJavaProcesses inspects container for running java processes
func queryJavaProcesses(shell string, pod *v1.Pod, container *v1.Container) ([]int, error) {
	logrus.Infof("Inspecting container '%s' for java processes", container.Name)

	stdout, err := ExecCommand(pod.Namespace, pod.Name, nil, container,
		shell, "-c", PS_CMD)

	if err != nil {
		logrus.Warnf("Failed to retrieve process list: %v", err)
		return nil, err
	} else {

		javaProcIds, extractErr := extractMatchedPids(stdout, JAVA_PROC_MATCHER)
		if extractErr != nil {
			logrus.Warnf("Failed to extract java processes: %v", extractErr)
			return nil, err
		}

		logrus.Infof("Java processes: %v", javaProcIds)

		return javaProcIds, nil
	}
}
