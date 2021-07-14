package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rookoutv1alpha1 "github.com/rookout/rookout-k8s-operator/api/v1alpha1"
)

const (
	DefaultRequeueAfter                 = 10 * time.Second
	DefaultInitContainerName            = "agent-init-container"
	DefaultInitContainerImage           = "docker.io/rookout/k8s-operator-init-container:latest"
	DefaultInitContainerImagePullPolicy = core.PullAlways
	DefaultSharedVolumeName             = "rookout-agent-shared-volume"
	DefaultSharedVolumeMountPath        = "/rookout"
	RookoutEnvVarPreffix                = "ROOKOUT_"
	RookoutTokenEnvVar                  = "ROOKOUT_TOKEN"
	RookoutControllerHostEnvVar         = "ROOKOUT_CONTROLLER_HOST"
)

type RookoutReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	DeploymentsManager DeploymentsManager
}

type OperatorConfiguration struct {
	rookoutv1alpha1.Rookout
	isReady bool
}

var configuration = OperatorConfiguration{isReady: false}

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
			r.syncDeployments(ctx)
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
					r.DeploymentsManager.ForgetDeployment(req.NamespacedName)
					logrus.Errorf("Deployment not found, maybe already deleted. deployment: %s", req.NamespacedName)
				}
				return ctrl.Result{}, nil
			}
			err = r.syncDeployment(ctx, &deployment)
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
	configuration.Spec.InitContainer.ImagePullPolicy = core.PullPolicy(getConfigStr(string(config.Spec.InitContainer.ImagePullPolicy), string(DefaultInitContainerImagePullPolicy)))
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

func (r *RookoutReconciler) syncDeployment(ctx context.Context, deployment *apps.Deployment) error {
	matchFound := false

	originalDeployment := client.MergeFrom(deployment.DeepCopy())

	var updatedContainers []core.Container
	for _, container := range deployment.Spec.Template.Spec.Containers {

		logrus.Infof("Validating container %s of deployment %s in %s namespace", container.Name, deployment.Name, deployment.GetNamespace())
		containerMatched := false
		for _, matcher := range configuration.Spec.Matchers {
			if deploymentMatch(matcher, *deployment) && containerMatch(matcher, container) && namespaceMatch(matcher, *deployment) && labelsMatch(matcher, *deployment) {
				containerMatched = true
				setRookoutEnvVars(&container.Env, matcher.EnvVars)
				break
			}
		}

		if !containerMatched {
			continue
		}

		container.Env = r.updateContainerEnvVars(container)

		container.VolumeMounts = append(container.VolumeMounts, core.VolumeMount{
			Name:      configuration.Spec.InitContainer.SharedVolumeName,
			MountPath: configuration.Spec.InitContainer.SharedVolumeMountPath,
		})

		updatedContainers = append(updatedContainers, container)
		matchFound = true
	}

	if !matchFound {
		var err error = nil

		if r.DeploymentsManager.IsDeploymentMarkedAsPatched(*deployment) || doesDeploymentHaveJavaSDKContainer(deployment) {
			err = r.unpatchDeployment(ctx, deployment, originalDeployment)

			if err != nil {
				logrus.Infof("Successfully removed java SDK from %s", deployment.Namespace+"/"+deployment.Name)
			}
		}

		r.DeploymentsManager.MarkDeploymentAsPatched(*deployment)
		return err
	}

	// Edge case - on first run, deployments might be patched but not registered in r.DeploymentsManager
	if doesDeploymentHaveJavaSDKContainer(deployment) {
		r.DeploymentsManager.MarkDeploymentAsNotPatched(*deployment)
		return nil
	}

	// Patching Deployment
	logrus.Infof("Adding rookout agent to deployment %s in %s namespace", deployment.Name, deployment.GetNamespace())
	deployment.Spec.Template.Spec.Containers = updatedContainers

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, core.Volume{
		Name:         configuration.Spec.InitContainer.SharedVolumeName,
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}},
	})

	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, core.Container{
		Image:           configuration.Spec.InitContainer.Image,
		ImagePullPolicy: configuration.Spec.InitContainer.ImagePullPolicy,
		Name:            configuration.Spec.InitContainer.ContainerName,
		VolumeMounts: []core.VolumeMount{
			{
				Name:      configuration.Spec.InitContainer.SharedVolumeName,
				MountPath: configuration.Spec.InitContainer.SharedVolumeMountPath},
		},
	})

	err := r.Client.Patch(ctx, deployment, originalDeployment)
	if err != nil {
		return err
	}

	r.DeploymentsManager.MarkDeploymentAsNotPatched(*deployment)
	logrus.Infof("Deployment %s patched successfully", deployment.Name)
	return nil
}

func doesDeploymentHaveJavaSDKContainer(deployment *apps.Deployment) bool {
	for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
		if initContainer.Name == configuration.Spec.InitContainer.ContainerName {
			return true
		}
	}

	return false
}

func (r *RookoutReconciler) updateContainerEnvVars(container core.Container) []core.EnvVar {
	newEnvVars := container.Env
	JavaAgent := fmt.Sprintf("-javaagent:%s/rook.jar", configuration.Spec.InitContainer.SharedVolumeMountPath)

	javaToolOptionsEnvVarIndex := -1

	for index, envVar := range container.Env {
		if envVar.Name == "JAVA_TOOL_OPTIONS" {
			javaToolOptionsEnvVarIndex = index
			break
		}
	}

	if javaToolOptionsEnvVarIndex == -1 {
		newEnvVars = append(newEnvVars, core.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: JavaAgent,
		})
	} else {
		newEnvVars[javaToolOptionsEnvVarIndex].Value = newEnvVars[javaToolOptionsEnvVarIndex].Value + " " + JavaAgent
	}

	return newEnvVars
}

func (r *RookoutReconciler) syncDeployments(ctx context.Context) {
	for _, deployment := range r.DeploymentsManager.Deployments {
		if !deployment.isPatched {
			r.syncDeployment(ctx, deployment.Deployment)
		}
	}
}

func (r *RookoutReconciler) unpatchDeployment(ctx context.Context, deployment *apps.Deployment, patchObj client.Patch) error {
	var updatedContainers []core.Container
	var updatedInitContainers []core.Container
	var updatedVolumes []core.Volume

	// Cleaning Env vars & volumeMounts per container
	for _, container := range deployment.Spec.Template.Spec.Containers {
		var updatedEnvVars []core.EnvVar
		var updatedVolumeMounts []core.VolumeMount

		for _, envVar := range container.Env {
			if envVar.Name == "JAVA_TOOL_OPTIONS" {
				javaToolOptions := strings.Split(envVar.Value, " ")
				javaToolOptions = removeElementWithSuffix(javaToolOptions, "rook.jar")

				if len(javaToolOptions) == 0 {
					continue
				}

				envVar.Value = strings.Join(javaToolOptions, " ")
			}

			if strings.HasPrefix(envVar.Name, RookoutEnvVarPreffix) {
				continue
			}

			updatedEnvVars = append(updatedEnvVars, envVar)
		}

		for _, volumeMount := range container.VolumeMounts {
			if volumeMount.Name != configuration.Spec.InitContainer.SharedVolumeName {
				updatedVolumeMounts = append(updatedVolumeMounts, volumeMount)
			}
		}

		container.Env = updatedEnvVars
		container.VolumeMounts = updatedVolumeMounts
		updatedContainers = append(updatedContainers, container)
	}

	// Removing Rookout volume and init container
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != configuration.Spec.InitContainer.SharedVolumeName {
			updatedVolumes = append(updatedVolumes, volume)
		}
	}

	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		if container.Name != configuration.Spec.InitContainer.ContainerName {
			updatedInitContainers = append(updatedInitContainers, container)
		}
	}

	deployment.Spec.Template.Spec.Containers = updatedContainers
	deployment.Spec.Template.Spec.InitContainers = updatedInitContainers
	deployment.Spec.Template.Spec.Volumes = updatedVolumes

	return r.Client.Patch(ctx, deployment, patchObj)
}
