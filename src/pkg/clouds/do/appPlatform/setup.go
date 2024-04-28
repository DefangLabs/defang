package appPlatform

import (
	"context"
	"fmt"
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
	Client      *godo.Client
	Region      do.Region
	ProjectName string
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
		Client:      godo.NewClient(client),
		Region:      region,
		ProjectName: stack,
	}

}

func (d DoApp) SetUp(ctx context.Context, services []*godo.AppServiceSpec, jobs []*godo.AppJobSpec) error {
	fmt.Printf("PROJECT NAME: %s", d.ProjectName)
	request := &godo.AppCreateRequest{
		Spec: &godo.AppSpec{
			Name:     d.ProjectName,
			Services: services,
			Jobs:     jobs,
		},
	}

	//appService := &godo.AppsServiceOp{}
	app, resp, err := d.Client.Apps.Create(ctx, request)

	fmt.Println(err)
	fmt.Println(app.ID)
	fmt.Println(resp.StatusCode)
	if err != nil {
		return err
	}

	return nil
}
