package controllers

import (
	rookout "github.com/rookout/rookout-k8s-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"testing"
)

func TestMatcher(t *testing.T) {
	assert := require.New(t)

	deployment := apps.Deployment{}
	deployment.Labels = map[string]string{"app": "right_app"}
	deployment.Name = "right-deployment"
	deployment.Spec.Template.Spec.Containers = []v1.Container{
		{
			Name: "right-container",
		},
	}

	rightMatcher := rookout.Matcher{
		Deployment: "right-deployment",
		Container:  "right-container",
		Labels:     map[string]string{"app": "right_app"},
	}
	wrongMatcher := rookout.Matcher{
		Deployment: "wrong-deployment",
		Container:  "wrong-container",
		Labels:     map[string]string{"app": "wrong_app"},
	}

	assert.True(deploymentMatch(rightMatcher, deployment))
	assert.True(containerMatch(rightMatcher, deployment.Spec.Template.Spec.Containers[0]))
	assert.True(labelsMatch(rightMatcher, deployment))

	assert.False(deploymentMatch(wrongMatcher, deployment))
	assert.False(containerMatch(wrongMatcher, deployment.Spec.Template.Spec.Containers[0]))
	assert.False(labelsMatch(wrongMatcher, deployment))

}

func TestEnvVarSet(t *testing.T) {
	assert := require.New(t)

	goodEnvVar := v1.EnvVar{Name: "ROOKOUT_VAR", Value: "should_set"}
	badEnvVar := v1.EnvVar{Name: "BAD_VAR", Value: "should_set"}

	envVars := []v1.EnvVar{goodEnvVar, badEnvVar}

	var actualVars []v1.EnvVar
	setRookoutEnvVars(&actualVars, envVars)

	assert.Equal(actualVars, []v1.EnvVar{goodEnvVar})
}
