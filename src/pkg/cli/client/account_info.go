package client

import "fmt"

type AccountInfo struct {
	AccountID string
	Details   string
	Provider  ProviderID
	Region    string
}

func (a AccountInfo) String() string {
	return fmt.Sprintf("%s account %q in region %q", a.Provider.Name(), a.AccountID, a.Region)
}
