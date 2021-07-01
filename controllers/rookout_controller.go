package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

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
	isReady bool
}

var configuration = OperatorConfiguration{isReady: false}

const (
	DefaultRequeueAfter                 = 10 * time.Second
	DefaultInitContainerName            = "agent-init-container"
	DefaultInitContainerImage           = "us.gcr.io/rookout/rookout-k8s-operator-init-container:1.0"
	DefaultInitContainerImagePullPolicy = v1.PullAlways
	DefaultSharedVolumeName             = "rookout-agent-shared-volume"
	DefaultSharedVolumeMountPath        = "/rookout"
	RookoutEnvVarPreffix                = "ROOKOUT_"
	RookoutTokenEnvVar                  = "ROOKOUT_TOKEN"
	RookoutControllerHostEnvVar         = "ROOKOUT_CONTROLLER_HOST"
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
	case OperatorConfigurationResource:
		{
			operatorConfiguration := rookoutv1alpha1.Rookout{}
			err := r.Client.Get(ctx, req.NamespacedName, &operatorConfiguration)
			if err != nil {
				return ctrl.Result{}, err
			}

			r.updateOperatorConfiguration(operatorConfiguration)
		}

	case DeploymentResource:
		{
			if !configuration.isReady {
				return ctrl.Result{Requeue: true, RequeueAfter: configuration.Spec.RequeueAfter}, nil
			}

			deployment := apps.Deployment{}
			err := r.Client.Get(ctx, req.NamespacedName, &deployment)
			if err != nil {
				if !strings.Contains(err.Error(), "not found") {
					logrus.Errorf("Deployment not found, maybe already deleted. deployment: %s", req.NamespacedName)
				}
				return ctrl.Result{}, nil
			}

			err = r.patchDeployment(ctx, &deployment)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
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
	configuration.isReady = false
	configuration.Spec.Matchers = config.Spec.Matchers
	configuration.Spec.InitContainer.Image = getConfigStr(config.Spec.InitContainer.Image, DefaultInitContainerImage)
	configuration.Spec.InitContainer.ImagePullPolicy = v1.PullPolicy(getConfigStr(string(config.Spec.InitContainer.ImagePullPolicy), string(DefaultInitContainerImagePullPolicy)))
	configuration.Spec.InitContainer.ContainerName = getConfigStr(config.Spec.InitContainer.ContainerName, DefaultInitContainerName)
	configuration.Spec.InitContainer.SharedVolumeMountPath = getConfigStr(config.Spec.InitContainer.SharedVolumeMountPath, DefaultSharedVolumeMountPath)
	configuration.Spec.InitContainer.SharedVolumeName = getConfigStr(config.Spec.InitContainer.SharedVolumeMountPath, DefaultSharedVolumeName)

	if config.Spec.RequeueAfter > 0 {
		configuration.Spec.RequeueAfter = config.Spec.RequeueAfter
	} else {
		configuration.Spec.RequeueAfter = DefaultRequeueAfter
	}

	if len(configuration.Spec.Matchers) == 0 {
		logrus.Error("No matchers found in configuration")
		return
	}

	for _, matcher := range configuration.Spec.Matchers {
		rookoutTokenFound := false
		onPremControllerFound := false

		for _, envVar := range matcher.EnvVars {
			if envVar.Name == RookoutTokenEnvVar {
				rookoutTokenFound = true
			}

			if envVar.Name == RookoutControllerHostEnvVar {
				onPremControllerFound = true
			}

			if onPremControllerFound && rookoutTokenFound {
				break
			}
		}

		if !rookoutTokenFound && !onPremControllerFound {
			logrus.Infof("Are you trying to connect to a deployed Rookout controller? if so, use %s and if you don't, use %s. See our docs at docs.rookout.com\"t", RookoutControllerHostEnvVar, RookoutTokenEnvVar)
			return
		}
	}

	configuration.isReady = true
	logrus.Info("Operator configuration updated")
}

func (r *RookoutReconciler) patchDeployment(ctx context.Context, deployment *apps.Deployment) error {
	shouldPatch := false

	for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
		if initContainer.Name == configuration.Spec.InitContainer.ContainerName {
			return nil
		}
	}

	originalDeployment := client.MergeFrom(deployment.DeepCopy())

	var updatedContainers []v1.Container
	for _, container := range deployment.Spec.Template.Spec.Containers {

		containerMatched := false
		for _, matcher := range configuration.Spec.Matchers {
			if deploymentMatch(matcher, *deployment) && containerMatch(matcher, container) && namespaceMatch(matcher, *deployment) && labelsMatch(matcher, *deployment) {
				setRookoutEnvVars(&container.Env, matcher.EnvVars)
				containerMatched = true
				break
			}
		}

		if !containerMatched {
			continue
		}

		logrus.Infof("Adding rookout agent to container %s of deployment %s in %s namespace", container.Name, deployment.Name, deployment.GetNamespace())

		container.Env = append(container.Env, v1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: fmt.Sprintf("-javaagent:%s/rook.jar %s", configuration.Spec.InitContainer.SharedVolumeMountPath, getJavaToolOptions((container.Env))),
		})

		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      configuration.Spec.InitContainer.SharedVolumeName,
			MountPath: configuration.Spec.InitContainer.SharedVolumeMountPath,
		})

		updatedContainers = append(updatedContainers, container)
		shouldPatch = true
	}

	if !shouldPatch {
		return nil
	}

	deployment.Spec.Template.Spec.Containers = updatedContainers

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, v1.Volume{
		Name:         configuration.Spec.InitContainer.SharedVolumeName,
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
	})

	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, v1.Container{
		Image:           configuration.Spec.InitContainer.Image,
		ImagePullPolicy: configuration.Spec.InitContainer.ImagePullPolicy,
		Name:            configuration.Spec.InitContainer.ContainerName,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      configuration.Spec.InitContainer.SharedVolumeName,
				MountPath: configuration.Spec.InitContainer.SharedVolumeMountPath},
		},
	})

	err := r.Client.Patch(ctx, deployment, originalDeployment)
	if err != nil {
		return err
	}

	logrus.Infof("Deployment %s patched successfully", deployment.Name)
	return nil
}
