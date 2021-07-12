package controllers

import (
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// We are saving a state of all running deployments in the cluster
type DeploymentsManager struct {
	Deployments map[types.UID]*RunningDeployment
}

type RunningDeployment struct {
	*apps.Deployment
	isPatched bool
}

func NewDeploymentsManager() DeploymentsManager {
	return DeploymentsManager{Deployments: make(map[types.UID]*RunningDeployment, 0)}
}

func (d *DeploymentsManager) addNonPatchedDeployment(deployment apps.Deployment) {
	d.Deployments[deployment.UID] = &RunningDeployment{
		Deployment: deployment.DeepCopy(),
		isPatched:  false,
	}
}

func (d *DeploymentsManager) addPatchedDeployment(deployment apps.Deployment) {
	d.Deployments[deployment.UID] = &RunningDeployment{
		Deployment: deployment.DeepCopy(),
		isPatched:  true,
	}
}

func (d *DeploymentsManager) isDeploymentPatched(deployment apps.Deployment) bool {
	return d.Deployments[deployment.UID].isPatched
}
