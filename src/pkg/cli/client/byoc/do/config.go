package do

import (
	"strings"

	"github.com/digitalocean/godo"
)

func deleteEnvVars(toDelete string, envVars *[]*godo.AppVariableDefinition) {

	var finalVars []*godo.AppVariableDefinition

	for _, envVar := range *envVars {
		if !strings.Contains(toDelete, envVar.Key) {
			finalVars = append(finalVars, envVar)
		}
	}
	*envVars = finalVars
}
