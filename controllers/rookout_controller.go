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
	REQUEUE_AFTER                          = 10 * time.Second
	DEFAULT_ROOKOUT_HOST                   = "wss://control.rookout.com"
	DEFAULT_ROOKOUT_PORT                   = "443"
	AGENT_INIT_CONTAINER_NAME              = "agent-init-container"
	AGENT_INIT_CONTAINER_IMAGE             = "us.gcr.io/rookout/rookout-k8s-operator-init-container:1.0"
	AGENT_INIT_CONTAINER_IMAGE_PULL_POLICY = v1.PullAlways
	AGENT_SHARED_VOLUEME_NAME              = "rookout-agent-shared-volume"
	AGENT_SHARED_VOLUEME_MOUNT_PATH        = "/rookout"
)

// Operator permissions
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/finalizers,verbs=update
//
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;patch
// +kubebuilder:rbac:groups="",resources="pods/exec",verbs=create

//  !!!! This Reconcile function handles both Rookout & Deployment resources !!!!
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

	deployment := apps.Deployment{}
	err = r.Client.Get(ctx, req.NamespacedName, &deployment)

	if err != nil {
		logrus.Infof("deployment not found - %s", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if OpState.PodsMatcher != "" && !strings.Contains(deployment.Name, OpState.PodsMatcher) {
		logrus.Infof("deployment %s not matched to \"%s\"", deployment.Name, OpState.PodsMatcher)
		return ctrl.Result{}, nil
	}

	err = r.addAgentViaInitContainer(ctx, &deployment)

	if err != nil {
		return ctrl.Result{}, err
	}

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

	var updatedContainers []v1.Container

	controllerHost := DEFAULT_ROOKOUT_HOST
	if OpState.ControllerHost != "" {
		controllerHost = OpState.ControllerHost
	}

	controllerPort := DEFAULT_ROOKOUT_PORT
	if OpState.ControllerPort != "" {
		controllerPort = OpState.ControllerPort
	}

	for _, container := range deployment.Spec.Template.Spec.Containers {
		container.Env = append(container.Env, v1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: fmt.Sprintf("-javaagent:%s/rook.jar", AGENT_SHARED_VOLUEME_MOUNT_PATH),
		},
			v1.EnvVar{
				Name:  "ROOKOUT_TOKEN",
				Value: OpState.RookoutToken,
			},
			v1.EnvVar{
				Name:  "ROOKOUT_CONTROLLER_HOST",
				Value: controllerHost,
			},
			v1.EnvVar{
				Name:  "ROOKOUT_CONTROLLER_PORT",
				Value: controllerPort,
			},
		)

		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      AGENT_SHARED_VOLUEME_NAME,
			MountPath: AGENT_SHARED_VOLUEME_MOUNT_PATH,
		})

		updatedContainers = append(updatedContainers, container)
	}

	deployment.Spec.Template.Spec.Containers = updatedContainers

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, v1.Volume{
		Name:         AGENT_SHARED_VOLUEME_NAME,
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
	})

	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, v1.Container{
		Image:           AGENT_INIT_CONTAINER_IMAGE,
		ImagePullPolicy: AGENT_INIT_CONTAINER_IMAGE_PULL_POLICY,
		Name:            AGENT_INIT_CONTAINER_NAME,
		VolumeMounts: []v1.VolumeMount{
			{Name: AGENT_SHARED_VOLUEME_NAME, MountPath: AGENT_SHARED_VOLUEME_MOUNT_PATH},
		},
	})

	err := r.Client.Patch(ctx, deployment, patch)
	if err != nil {
		return err
	}

	logrus.Infof("init container added to deployment %s", deployment.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RookoutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&source.Kind{Type: &apps.Deployment{}}, &handler.EnqueueRequestForObject{}).
		For(&rookoutv1alpha1.Rookout{}).
		Complete(r)
}
