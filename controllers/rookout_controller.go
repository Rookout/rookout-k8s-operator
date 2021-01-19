package controllers

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rookoutv1alpha1 "github.com/rookout/rookout-k8s-operator/api/v1alpha1"
)

type RookoutReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

type OperatorState struct {
	IsReady         bool
	RookoutToken    string
	PodsMatcher     string
	JavaProcMatcher string
	ControllerHost  string
	ControllerPort  string
}

var OpState = OperatorState{IsReady: false}

const (
	SRC_DIR                   = "/var/rookout"
	DST_DIR                   = "/var/rookout"
	PS_CMD                    = "ps -x"
	DEFAULT_JAVA_PROC_MATCHER = "java -jar"
	// TODO : detect java version before adding flags since those flags not supported on java 7
	// JAVA_FLAGS                = "--add-opens java.base/java.net=ALL-UNNAMED" // disable reflection warning. ref : https://nipafx.dev/five-command-line-options-hack-java-module-system/ "
	INJECTION_SUCCESS_LOG                  = "Injected successfully"
	REQUEUE_AFTER                          = 10 * time.Second
	DEFAULT_ROOKOUT_HOST                   = "wss://control.rookout.com"
	DEFAULT_ROOKOUT_PORT                   = "443"
	AGENT_INIT_CONTAINER_NAME              = "agent-init-container"
	AGENT_INIT_CONTAINER_IMAGE             = "us.gcr.io/rookout/rookout-k8s-operator-init-container:1.0"
	AGENT_INIT_CONTAINER_IMAGE_PULL_POLICY = v1.PullAlways
)

// Annotation for generating RBAC for operator's own resources
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/finalizers,verbs=update

// Annotation for generating RBAC to Watch  & Deployments
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;patch
// +kubebuilder:rbac:groups="",resources="pods/exec",verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *RookoutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// first check if the request is a Rookout resource
	rookoutResource := rookoutv1alpha1.Rookout{}
	err := r.Client.Get(ctx, req.NamespacedName, &rookoutResource)

	// if fetched successfully, handle it
	if err == nil && rookoutResource.Spec.Token != "" {
		OpState.IsReady = true
		OpState.RookoutToken = rookoutResource.Spec.Token
		OpState.PodsMatcher = rookoutResource.Spec.PodsMatcher
		OpState.JavaProcMatcher = rookoutResource.Spec.JavaProcMatcher
		OpState.ControllerHost = rookoutResource.Spec.ControllerHost
		OpState.ControllerPort = rookoutResource.Spec.ControllerPort

		return ctrl.Result{}, nil
	}

	if !OpState.IsReady {
		logrus.Infof("operator not ready yet. Requeue request for %v seconds", REQUEUE_AFTER)
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_AFTER}, nil
	}

	// if we reached here the request resource should be a POD (not Rookout resource)
	deployment := apps.Deployment{}
	err = r.Client.Get(ctx, req.NamespacedName, &deployment)

	if err != nil {
		logrus.Infof("deployment not found - %s", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	//if OpState.PodsMatcher != "" && !strings.Contains(pod.Name, OpState.PodsMatcher) {
	//	logrus.Infof("pod %s not matched to \"%s\"", pod.Name, OpState.PodsMatcher)
	//	return ctrl.Result{}, nil
	//}
	//
	//if len(pod.Status.ContainerStatuses) == 0 {
	//	logrus.Infof("namespace: %s, name: %s has no status", req.Namespace, req.Name)
	//	return ctrl.Result{}, nil
	//}
	//
	//// TODO: figure how to detect a terminating container which could also contain status ready
	//if !pod.Status.ContainerStatuses[0].Ready {
	//	logrus.Infof("namespace: %s, name: %s, status: not ready", req.Namespace, req.Name)
	//	return ctrl.Result{}, nil
	//}

	if OpState.PodsMatcher != "" && !strings.Contains(deployment.Name, OpState.PodsMatcher) {
		logrus.Infof("deployment %s not matched to \"%s\"", deployment.Name, OpState.PodsMatcher)
		return ctrl.Result{}, nil
	}

	err = r.addAgentViaInitContainer(ctx, &deployment)

	if err != nil {
		return ctrl.Result{}, err
	}

	//logrus.Infof("container statuses: %v", pod.Status.ContainerStatuses)

	return ctrl.Result{}, nil
}

func (r *RookoutReconciler) addAgentViaInitContainer(ctx context.Context, deployment *apps.Deployment) error {
	for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
		if initContainer.Name == AGENT_INIT_CONTAINER_NAME {
			logrus.Infof("init container already exists in deployment %s", deployment.Name)
			return nil
		}
	}

	patch := client.MergeFrom(deployment.DeepCopy())

	// TODO : add env var only for relevant containers
	for _, container := range deployment.Spec.Template.Spec.Containers {
		container.Env = append(container.Env, v1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: "-javaagent:/var/rookout/rook.jar",
		})
	}

	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, v1.Container{
		Image:           AGENT_INIT_CONTAINER_IMAGE,
		ImagePullPolicy: AGENT_INIT_CONTAINER_IMAGE_PULL_POLICY,
		Name:            AGENT_INIT_CONTAINER_NAME,
	})

	err := r.Client.Patch(ctx, deployment, patch)
	if err != nil {
		return err
	}

	logrus.Infof("init container added to deployment %s", deployment.Name)
	return nil
}

func (r *RookoutReconciler) InjectAgent(pod *v1.Pod) error {
	for _, container := range pod.Spec.Containers {
		podUtils, podUtilsErr := NewPodUtils(pod.Namespace, pod.Name, nil, &container)

		if podUtilsErr != nil {
			logrus.Errorf("failed to initialize pod utils for container %s : %v", container.Name, podUtilsErr)
			continue
		}

		javaProcMatcher := DEFAULT_JAVA_PROC_MATCHER
		if OpState.JavaProcMatcher != "" {
			javaProcMatcher = OpState.JavaProcMatcher
		}

		javaPids, pidErr := podUtils.QueryMatchedProcesses(javaProcMatcher)
		if pidErr != nil {
			logrus.WithField("err", pidErr).Errorf("failed to retrieve java processes from container %v", container.Name)
			continue
		}

		if len(javaPids) == 0 {
			continue
		}

		logrus.Infof("container: %s, java processes: %v", container.Name, javaPids)

		// TODO : delete copied files after injection
		copyErr := podUtils.CopyToPod(SRC_DIR, DST_DIR)
		if copyErr != nil {
			logrus.WithField("err", copyErr).Errorf("failed to copy rook loader to container %v", container.Name)
			continue
		}

		controllerHost := DEFAULT_ROOKOUT_HOST
		if OpState.ControllerHost != "" {
			controllerHost = OpState.ControllerHost
		}

		controllerPort := DEFAULT_ROOKOUT_PORT
		if OpState.ControllerPort != "" {
			controllerPort = OpState.ControllerPort
		}

		logrus.Infof("controller: %s:%s", controllerHost, controllerPort)

		for _, pid := range javaPids {
			// TODO : test what happens if wh inject twice
			loadCmd := fmt.Sprintf("ROOKOUT_LOG_LEVEL=DEBUG ROOKOUT_LOG_TO_STDERR=1 ROOKOUT_CONTROLLER_PORT=:%s ROOKOUT_CONTROLLER_HOST=%s ROOKOUT_TOKEN=%s ROOKOUT_TARGET_PID=%d java -jar %s/rook.jar", controllerPort, controllerHost, OpState.RookoutToken, pid, DST_DIR)
			stdout, execErr := podUtils.ExecCommand(true, loadCmd)

			if execErr != nil {
				logrus.WithFields(logrus.Fields{
					"err":    execErr,
					"stdout": stdout,
				}).Errorf("failed to inject rook to pid %d", pid)
				continue
			}

			if !strings.Contains(stdout, INJECTION_SUCCESS_LOG) {
				logrus.WithField("stdout", stdout).Errorf("failed to inject rook to pid %d (no success log found)", pid)
				continue
			}

			logrus.Info(stdout)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RookoutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		//Watches(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &apps.Deployment{}}, &handler.EnqueueRequestForObject{}).
		For(&rookoutv1alpha1.Rookout{}).
		Complete(r)
}
