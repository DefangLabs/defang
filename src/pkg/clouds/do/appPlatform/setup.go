package appPlatform

import (
	"context"
	"github.com/defang-io/defang/src/pkg/clouds/do"
	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
	"os"
)

type DoAppPlatform struct {
	DoApp
	appName string
}

type DoApp struct {
	Client *godo.Client
	Region do.Region
}

func New(stack string, region do.Region) *DoApp {
	if stack == "" {
		panic("stack must be set")
	}
	pat := os.Getenv("DO_PAT")
	if pat == "" {
		panic("digital ocean pat must be set")
	}
	tokenSource := &oauth2.Token{AccessToken: pat}
	client := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(tokenSource))

	return &DoApp{
		Client: godo.NewClient(client),
		Region: region,
	}

}
