package controllers

import (
	"github.com/rookout/rookout-k8s-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	v12 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
)

func setRookoutEnvVars(env *[]v1.EnvVar, evnVars []v1.EnvVar) {
	for _, envVar := range evnVars {
		if !strings.HasPrefix(envVar.Name, ROOKOUT_ENV_VAR_PREFFIX) {
			logrus.Warnf("%s is not a valid env variable. Only vars with %s prefix allowed.", envVar.Name, ROOKOUT_ENV_VAR_PREFFIX)
			continue
		}

		*env = append(*env, envVar)
	}
}

func labelsMatch(matcher v1alpha1.Matcher, deployment v12.Deployment) bool {
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

func deploymentMatch(matcher v1alpha1.Matcher, deployment v12.Deployment) bool {
	return matcher.Deployment == "" || strings.Contains(deployment.Name, matcher.Deployment)
}

func containerMatch(matcher v1alpha1.Matcher, container v1.Container) bool {
	return matcher.Container == "" || strings.Contains(container.Name, matcher.Container)
}

const (
	OPERATOR_CONFIGURATUION_REQUEST = "Rookout"
	DEPLOYMENT_REQUEST              = "Deployment"
)

func getResourceType(req ctrl.Request) string {
	if strings.HasPrefix(req.Name, OPERATOR_CONFIGURATUION_REQUEST) {
		return OPERATOR_CONFIGURATUION_REQUEST
	}

	if strings.HasPrefix(req.Name, DEPLOYMENT_REQUEST) {
		return DEPLOYMENT_REQUEST
	}

	return req.Name
}

func getConfigStr(config string, defaultValue string) string {
	if config != "" {
		return config
	}

	return defaultValue
}
