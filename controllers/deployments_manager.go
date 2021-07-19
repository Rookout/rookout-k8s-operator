package controllers

import (
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// We are saving a state of all running deployments in the cluster
type DeploymentsManager struct {
	Deployments map[string]*RunningDeployment
}

type RunningDeployment struct {
	*apps.Deployment
	isPatched bool
}

func NewDeploymentsManager() DeploymentsManager {
	return DeploymentsManager{Deployments: make(map[string]*RunningDeployment, 0)}
}

func createDeploymentKey(deployment apps.Deployment) string {
	return types.NamespacedName{
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
	}.String()
}

func (d *DeploymentsManager) MarkDeploymentAsPatched(deployment apps.Deployment) {
	d.Deployments[createDeploymentKey(deployment)] = &RunningDeployment{
		Deployment: deployment.DeepCopy(),
		isPatched:  false,
	}
}

func (d *DeploymentsManager) MarkDeploymentAsNotPatched(deployment apps.Deployment) {
	d.Deployments[createDeploymentKey(deployment)] = &RunningDeployment{
		Deployment: deployment.DeepCopy(),
		isPatched:  true,
	}
}

func (d *DeploymentsManager) ForgetDeployment(namespacedName types.NamespacedName) {
	key := namespacedName.String()
	if _, ok := d.Deployments[key]; ok {
		delete(d.Deployments, key)
	}
}

func (d *DeploymentsManager) IsDeploymentMarkedAsPatched(deployment apps.Deployment) bool {
	mappedDeployment, exist := d.Deployments[createDeploymentKey(deployment)]

	return exist && mappedDeployment.isPatched
}
