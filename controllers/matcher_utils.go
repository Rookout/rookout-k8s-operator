package controllers

import (
	"strings"

	"github.com/rookout/rookout-k8s-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func setRookoutEnvVars(env *[]core.EnvVar, evnVars []core.EnvVar) {
	for _, envVar := range evnVars {
		if !strings.HasPrefix(envVar.Name, RookoutEnvVarPreffix) {
			logrus.Warnf("%s is not a valid env variable. Only vars with %s prefix allowed.", envVar.Name, RookoutEnvVarPreffix)
			continue
		}

		*env = append(*env, envVar)
	}
}

func labelsMatch(matcher v1alpha1.Matcher, deployment apps.Deployment) bool {
	for expectedLabelName, expectedLabelValue := range matcher.Labels {
		labelMatched := false

		for labelName, labelValue := range deployment.Labels {
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

func namespaceMatch(matcher v1alpha1.Matcher, deployment apps.Deployment) bool {
	return matcher.Namespace == "" || strings.Contains(deployment.GetNamespace(), matcher.Namespace)
}

func deploymentMatch(matcher v1alpha1.Matcher, deployment apps.Deployment) bool {
	return matcher.Deployment == "" || strings.Contains(deployment.Name, matcher.Deployment)
}

func containerMatch(matcher v1alpha1.Matcher, container core.Container) bool {
	return matcher.Container == "" || strings.Contains(container.Name, matcher.Container)
}

const (
	OperatorConfigurationResource = "Rookout"
	DeploymentResource            = "Deployment"
	ConfigurationResourceName     = "rookout-operator-configuration"
)

func getResourceType(req ctrl.Request) string {
	if req.Name == ConfigurationResourceName {
		return OperatorConfigurationResource
	}

	return DeploymentResource
}

func getConfigStr(config string, defaultValue string) string {
	if config != "" {
		return config
	}

	return defaultValue
}

func removeElementWithSuffix(s []string, suffix string) []string {
	for i, v := range s {
		if strings.HasSuffix(v, suffix) {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}
