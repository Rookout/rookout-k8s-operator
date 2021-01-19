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
	Matchers       rookoutv1alpha1.Matchers
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

// Operator permissions
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/finalizers,verbs=update
//
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;patch
// +kubebuilder:rbac:groups="",resources="pods/exec",verbs=create

// TODO : document what we support in the docs - currently only deployments

//  !!!! This Reconcile function handles both Rookout & Deployment resources !!!!
func (r *RookoutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// first check if the request is a Rookout resource
	rookoutResource := rookoutv1alpha1.Rookout{}
	err := r.Client.Get(ctx, req.NamespacedName, &rookoutResource)

	// if fetched successfully, handle it
	if err == nil {
		OpState.IsReady = true
		OpState.RookoutEnvVars = rookoutResource.Spec.RookoutEnvVars
		OpState.Matchers = rookoutResource.Spec.Matchers
		return ctrl.Result{}, nil
	}

	if !OpState.IsReady {
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_AFTER}, nil
	}

	deployment := apps.Deployment{}
	err = r.Client.Get(ctx, req.NamespacedName, &deployment)

	if err != nil {
		logrus.Infof("deployment not found - %s", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if isDeploymentMatched(deployment) {
		return ctrl.Result{}, nil
	}

	err = r.addAgentViaInitContainer(ctx, &deployment)

	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func labelsContains(expectedLabels, actualLabels map[string]string) bool {
	for expectedLabelName, expectedLabelValue := range expectedLabels {
		labelMatched := false

		for labelName, labelValue := range actualLabels {
			if labelName == expectedLabelName && labelValue == expectedLabelValue {
				labelMatched = true
				break
			}
		}

		if !labelMatched {
			return false
		}
	}

	return true
}

func isDeploymentMatched(deployment apps.Deployment) bool {
	if OpState.Matchers.DeploymentMatcher.Matcher != "" && !strings.Contains(deployment.Name, OpState.Matchers.DeploymentMatcher.Matcher) {
		return false
	}

	if OpState.Matchers.LabelsMatcher.Matcher != nil {
		return labelsContains(OpState.Matchers.LabelsMatcher.Matcher, deployment.Labels)
	}

	return true
}

func isContainerMatched(container v1.Container) bool {
	if OpState.Matchers.ContainerMatcher.Matcher != "" && !strings.Contains(container.Name, OpState.Matchers.ContainerMatcher.Matcher) {
		return false
	}

	return true
}

func setRookoutEnvVars(env *[]v1.EnvVar, evnVars []v1.EnvVar) {
	for _, envVar := range evnVars {
		if !strings.HasPrefix(envVar.Name, ROOKOUT_ENV_VAR_PREFFIX) {
			logrus.Warnf("%s is not a valid env variable because its lack of %s prefix.", ROOKOUT_ENV_VAR_PREFFIX, envVar.Name)
			continue
		}

		*env = append(*env, envVar)
	}
}

func (r *RookoutReconciler) addAgentViaInitContainer(ctx context.Context, deployment *apps.Deployment) error {
	shouldPatch := false

	for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
		if initContainer.Name == AGENT_INIT_CONTAINER_NAME {
			return nil
		}
	}

	patch := client.MergeFrom(deployment.DeepCopy())

	var updatedContainers []v1.Container

	for _, container := range deployment.Spec.Template.Spec.Containers {

		if !isContainerMatched(container) {
			continue
		}

		if OpState.Matchers.ContainerMatcher.EnvVars != nil {
			setRookoutEnvVars(&container.Env, OpState.Matchers.ContainerMatcher.EnvVars)
		}

		if OpState.Matchers.DeploymentMatcher.EnvVars != nil {
			setRookoutEnvVars(&container.Env, OpState.Matchers.DeploymentMatcher.EnvVars)
		}

		if OpState.RookoutEnvVars != nil {
			setRookoutEnvVars(&container.Env, OpState.RookoutEnvVars)
		}

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

	logrus.Infof("init container added to deployment %s", deployment.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RookoutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// TODO : handle deployment events in dedicated reconcile func
		Watches(&source.Kind{Type: &apps.Deployment{}}, &handler.EnqueueRequestForObject{}).
		For(&rookoutv1alpha1.Rookout{}).
		Complete(r)
}
