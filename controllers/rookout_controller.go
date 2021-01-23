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
	IsReady        bool
	RookoutEnvVars []v1.EnvVar
	Matchers       []rookoutv1alpha1.Matcher
}

var OpState = OperatorState{IsReady: false}

const (
	REQUEUE_AFTER                          = 10 * time.Second
	AGENT_INIT_CONTAINER_NAME              = "agent-init-container"
	AGENT_INIT_CONTAINER_IMAGE             = "us.gcr.io/rookout/rookout-k8s-operator-init-container:1.0"
	AGENT_INIT_CONTAINER_IMAGE_PULL_POLICY = v1.PullAlways
	AGENT_SHARED_VOLUEME_NAME              = "rookout-agent-shared-volume"
	AGENT_SHARED_VOLUEME_MOUNT_PATH        = "/rookout"
	ROOKOUT_ENV_VAR_PREFFIX                = "ROOKOUT_"
)

// !!!!!!!!!!!!!!!!!!!!
// Operator permissions - make sure we don't have unused permissions here
// !!!!!!!!!!!!!!!!!!!!
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/finalizers,verbs=update
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;patch

// This Reconcile function handles the following resources:
// - Rookout
// - Deployment
//
// the common design pattern is that each reconcile should handle only one resource type
func (r *RookoutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	operatorConfiguration := rookoutv1alpha1.Rookout{}
	err := r.Client.Get(ctx, req.NamespacedName, &operatorConfiguration)

	if err == nil {
		OpState.IsReady = true
		OpState.RookoutEnvVars = operatorConfiguration.Spec.RookoutEnvVars
		OpState.Matchers = operatorConfiguration.Spec.Matchers
		logrus.Info("operator configuration updated")
		return ctrl.Result{}, nil
	}

	if !OpState.IsReady {
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_AFTER}, nil
	}

	deployment := apps.Deployment{}
	err = r.Client.Get(ctx, req.NamespacedName, &deployment)

	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			logrus.Errorf("error during deployment fetch - %s", err.Error())
		}
		return ctrl.Result{}, nil
	}

	err = r.patchDeployment(ctx, &deployment)

	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RookoutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&source.Kind{Type: &apps.Deployment{}}, &handler.EnqueueRequestForObject{}).
		For(&rookoutv1alpha1.Rookout{}).
		Complete(r)
}

func (r *RookoutReconciler) patchDeployment(ctx context.Context, deployment *apps.Deployment) error {
	shouldPatch := false

	for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
		if initContainer.Name == AGENT_INIT_CONTAINER_NAME {
			return nil
		}
	}

	patch := client.MergeFrom(deployment.DeepCopy())

	var updatedContainers []v1.Container
	for _, container := range deployment.Spec.Template.Spec.Containers {

		containerMatched := false
		for _, matcher := range OpState.Matchers {
			if deploymentMatch(matcher, *deployment) && containerMatch(matcher, container) && labelsMatch(matcher, *deployment) {
				setRookoutEnvVars(&container.Env, OpState.RookoutEnvVars)
				setRookoutEnvVars(&container.Env, matcher.EnvVars)
				containerMatched = true
				break
			}
		}

		if !containerMatched {
			continue
		}

		logrus.Infof("adding rookout agent to container %s of deployment %s", container.Name, deployment.Name)

		container.Env = append(container.Env, v1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: fmt.Sprintf("-javaagent:%s/rook.jar", AGENT_SHARED_VOLUEME_MOUNT_PATH),
		})

		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      AGENT_SHARED_VOLUEME_NAME,
			MountPath: AGENT_SHARED_VOLUEME_MOUNT_PATH,
		})

		updatedContainers = append(updatedContainers, container)
		shouldPatch = true
	}

	if !shouldPatch {
		return nil
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

	logrus.Infof("deployment %s patched successfully", deployment.Name)
	return nil
}
