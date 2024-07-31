package appPlatform

import (
	"context"
	"errors"
	"fmt"
	"github.com/DefangLabs/defang/src/pkg/clouds/do"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/ptr"
	"github.com/digitalocean/godo"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"os"
	"regexp"
)

const (
	DockerHub = "DOCKER_HUB"
	Docr      = "DOCR"
	CDName    = "defang-cd"
)

type DoAppPlatform struct {
	DoApp
	appName string
}

type DoApp struct {
	Client      *godo.Client
	Region      do.Region
	ProjectName string
	BucketName  string
	AppID       string
}

func New(stack string, region do.Region) *DoApp {
	if stack == "" {
		panic("stack must be set")
	}

	client := DoApp{}.newClient(context.Background())

	return &DoApp{
		Client:      client,
		Region:      region,
		ProjectName: stack,
		BucketName:  "defang-test",
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

func (d DoApp) Run(ctx context.Context, env []*godo.AppVariableDefinition, cmd string) (string, error) {
	client := d.newClient(ctx)

	appJobSpec := &godo.AppSpec{
		Name: CDName,
		Jobs: []*godo.AppJobSpec{{
			Name: fmt.Sprintf("defang-cd-%s", d.ProjectName),
			Envs: env,
			Image: &godo.ImageSourceSpec{
				RegistryType: Docr,
				Repository:   "defangmvp/do-cd",
			},
			InstanceCount:    1,
			InstanceSizeSlug: "basic-xs",
			RunCommand:       cmd,
		}},
	}

	var currentCd = &godo.App{}

	appList, _, err := client.Apps.List(ctx, &godo.ListOptions{})

	if err != nil {
		return "", nil
	}

	for _, app := range appList {
		if app.Spec.Name == CDName {
			currentCd = app
		}
	}

	//Update current CD app if it exists
	if currentCd.Spec != nil && currentCd.Spec.Name != "" {
		currentCd, _, err = client.Apps.Update(ctx, currentCd.ID, &godo.AppUpdateRequest{
			Spec: appJobSpec,
		})
	} else {
		currentCd, _, err = client.Apps.Create(ctx, &godo.AppCreateRequest{
			Spec: appJobSpec,
		})
	}

	if err != nil {
		return "", err
	}

	return currentCd.ID, err
}

func (d DoApp) newClient(ctx context.Context) *godo.Client {
	pat := os.Getenv("DO_PAT")
	if pat == "" {
		panic("digital ocean pat must be set")
	}
	tokenSource := &oauth2.Token{AccessToken: pat}
	client := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(tokenSource))
	return godo.NewClient(client)
}

var s3InvalidCharsRegexp = regexp.MustCompile(`[^a-zA-Z0-9!_.*'()-]`)

func (d DoApp) CreateUploadURL(ctx context.Context, name string) (string, error) {

	term.Debug("Creating upload url for: ", name)
	s3Client := d.createS3Client()

	prefix := "uploads/"

	if name == "" {
		name = uuid.NewString()
	} else {
		if len(name) > 64 {
			return "", errors.New("name must be less than 64 characters")
		}
		// Sanitize the digest so it's safe to use as a file name
		name = s3InvalidCharsRegexp.ReplaceAllString(name, "_")
		// name = path.Join(buildsPath, tenantId.String(), digest); TODO: avoid collisions between tenants
	}

	// Use S3 SDK to create a presigned URL for uploading a file.
	req, err := s3.NewPresignClient(s3Client).PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: &d.BucketName,
		Key:    ptr.String(prefix + name),
	})

	if err != nil {
		return "", err
	}

	term.Debug(fmt.Sprintf("S3 URL: %s", req.URL))
	return req.URL, nil
}

func (d DoApp) CreateS3DownloadUrl(ctx context.Context, name string) (string, error) {

	s3Client := d.createS3Client()

	req, err := s3.NewPresignClient(s3Client).PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &d.BucketName,
		Key:    &name,
	})

	if err != nil {
		return "", err
	}

	return req.URL, nil

}

func (d DoApp) createS3Client() *s3.Client {
	id := os.Getenv("DO_SPACES_ID")
	key := os.Getenv("DO_SPACES_KEY")

	cfg := aws.Config{
		Credentials:  credentials.NewStaticCredentialsProvider(id, key, ""),
		BaseEndpoint: aws.String(fmt.Sprintf("https://%s.digitaloceanspaces.com", d.Region.String())),
		Region:       *aws.String(d.Region.String()),
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return s3Client
}
