package do

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/digitalocean/godo"
	"strings"
)

func deleteEnvVars(toDelete string, envVars *[]*godo.AppVariableDefinition) {

	var finalVars []*godo.AppVariableDefinition

	for _, envVar := range *envVars {
		if !strings.Contains(toDelete, envVar.Key) {
			term.Debugf("MATCH FOUND: %s", envVar.Key)
			finalVars = append(finalVars, envVar)
		}
	}
	*envVars = finalVars
}
