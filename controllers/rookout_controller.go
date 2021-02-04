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

type OperatorConfiguration struct {
	rookoutv1alpha1.Rookout
	IsReady bool
}

var Configuration = OperatorConfiguration{IsReady: false}

const (
	DEFAULT_REQUEUE_AFTER                    = 10 * time.Second
	DEFAULT_INIT_CONTAINER_NAME              = "agent-init-container"
	DEFAULT_INIT_CONTAINER_IMAGE             = "us.gcr.io/rookout/rookout-k8s-operator-init-container:1.0"
	DEFAULT_INIT_CONTAINER_IMAGE_PULL_POLICY = v1.PullAlways
	DEFAULT_SHARED_VOLUEME_NAME              = "rookout-agent-shared-volume"
	DEFAULT_SHARED_VOLUEME_MOUNT_PATH        = "/rookout"
	ROOKOUT_ENV_VAR_PREFFIX                  = "ROOKOUT_"
	ROOKOUT_TOKEN_ENV_VAR                    = "ROOKOUT_TOKEN"
)

// !!!!!!!!!!!!!!!!!!!!
// Operator permissions - make sure we don't have unused permissions here
// !!!!!!!!!!!!!!!!!!!!
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rookout.rookout.com,resources=rookouts/finalizers,verbs=update
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;list;patch

func (r *RookoutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	switch getResourceType(req) {
	case OPERATOR_CONFIGURATUION_RESOURCE:
		{
			operatorConfiguration := rookoutv1alpha1.Rookout{}
			err := r.Client.Get(ctx, req.NamespacedName, &operatorConfiguration)
			if err != nil {
				return ctrl.Result{}, err
			}

			r.updateOperatorConfiguration(operatorConfiguration)
		}

	case DEPLOYMENT_RESOURCE:
		{
			if !r.isReady() {
				return ctrl.Result{Requeue: true, RequeueAfter: Configuration.Spec.RequeueAfter}, nil
			}

			deployment := apps.Deployment{}
			err := r.Client.Get(ctx, req.NamespacedName, &deployment)
			if err != nil {
				if !strings.Contains(err.Error(), "not found") {
					logrus.Errorf("deployment not found - %s. maybe already deleted.", req.NamespacedName)
				}
				return ctrl.Result{}, nil
			}

			err = r.patchDeployment(ctx, &deployment)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

	default:
		logrus.Errorf("unknown request: %s", req.Name)
	}

	return ctrl.Result{}, nil
}

func (r *RookoutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Watches(&source.Kind{Type: &apps.Deployment{}}, &handler.EnqueueRequestForObject{}).
		For(&rookoutv1alpha1.Rookout{}).
		Complete(r)
}

func (r *RookoutReconciler) updateOperatorConfiguration(config rookoutv1alpha1.Rookout) {
	Configuration.IsReady = false
	Configuration.Spec.RookoutEnvVars = config.Spec.RookoutEnvVars
	Configuration.Spec.Matchers = config.Spec.Matchers
	Configuration.Spec.InitContainer.Image = getConfigStr(config.Spec.InitContainer.Image, DEFAULT_INIT_CONTAINER_IMAGE)
	Configuration.Spec.InitContainer.ImagePullPolicy = v1.PullPolicy(getConfigStr(string(config.Spec.InitContainer.ImagePullPolicy), string(DEFAULT_INIT_CONTAINER_IMAGE_PULL_POLICY)))
	Configuration.Spec.InitContainer.ContainerName = getConfigStr(config.Spec.InitContainer.ContainerName, DEFAULT_INIT_CONTAINER_NAME)
	Configuration.Spec.InitContainer.SharedVolumeMountPath = getConfigStr(config.Spec.InitContainer.SharedVolumeMountPath, DEFAULT_SHARED_VOLUEME_MOUNT_PATH)
	Configuration.Spec.InitContainer.SharedVolumeName = getConfigStr(config.Spec.InitContainer.SharedVolumeMountPath, DEFAULT_SHARED_VOLUEME_NAME)

	if config.Spec.RequeueAfter > 0 {
		Configuration.Spec.RequeueAfter = config.Spec.RequeueAfter
	} else {
		Configuration.Spec.RequeueAfter = DEFAULT_REQUEUE_AFTER
	}

	rookoutTokenFound := false
	for _, envVar := range Configuration.Spec.RookoutEnvVars {
		if envVar.Name == ROOKOUT_TOKEN_ENV_VAR {
			rookoutTokenFound = true
			break
		}
	}

	if !rookoutTokenFound {
		logrus.Info("rookout token env var not found in configuration")
		return
	}

	if len(Configuration.Spec.Matchers) == 0 {
		logrus.Info("no matchers found in configuration")
		return
	}

	Configuration.IsReady = true
	logrus.Info("operator configuration updated")
}

func (r *RookoutReconciler) isReady() bool {
	return Configuration.IsReady
}

func (r *RookoutReconciler) patchDeployment(ctx context.Context, deployment *apps.Deployment) error {
	shouldPatch := false

	for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
		if initContainer.Name == Configuration.Spec.InitContainer.ContainerName {
			return nil
		}
	}

	originalDeployment := client.MergeFrom(deployment.DeepCopy())

	var updatedContainers []v1.Container
	for _, container := range deployment.Spec.Template.Spec.Containers {

		containerMatched := false
		for _, matcher := range Configuration.Spec.Matchers {
			if deploymentMatch(matcher, *deployment) && containerMatch(matcher, container) && labelsMatch(matcher, *deployment) {
				setRookoutEnvVars(&container.Env, Configuration.Spec.RookoutEnvVars)
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
			Value: fmt.Sprintf("-javaagent:%s/rook.jar", Configuration.Spec.InitContainer.SharedVolumeMountPath),
		})

		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      Configuration.Spec.InitContainer.SharedVolumeName,
			MountPath: Configuration.Spec.InitContainer.SharedVolumeMountPath,
		})

		updatedContainers = append(updatedContainers, container)
		shouldPatch = true
	}

	if !shouldPatch {
		return nil
	}

	deployment.Spec.Template.Spec.Containers = updatedContainers

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, v1.Volume{
		Name:         Configuration.Spec.InitContainer.SharedVolumeName,
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
	})

	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, v1.Container{
		Image:           Configuration.Spec.InitContainer.Image,
		ImagePullPolicy: Configuration.Spec.InitContainer.ImagePullPolicy,
		Name:            Configuration.Spec.InitContainer.ContainerName,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      Configuration.Spec.InitContainer.SharedVolumeName,
				MountPath: Configuration.Spec.InitContainer.SharedVolumeMountPath},
		},
	})

	err := r.Client.Patch(ctx, deployment, originalDeployment)
	if err != nil {
		return err
	}

	logrus.Infof("deployment %s patched successfully", deployment.Name)
	return nil
}
