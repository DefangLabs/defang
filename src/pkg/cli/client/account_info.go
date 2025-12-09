package client

import "fmt"

type AccountInfo struct {
	AccountID string
	Details   string
	Provider  ProviderID
	Region    string
}

func (a AccountInfo) String() string {
	str := fmt.Sprintf("%s account %q", a.Provider.Name(), a.AccountID)
	if a.Region != "" {
		str += " in " + a.Region
	}
	return str
}
