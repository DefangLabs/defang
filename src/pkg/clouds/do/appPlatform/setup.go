package appPlatform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/do"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/ptr"
	"github.com/digitalocean/godo"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const (
	CdImageBase = "defangio/cd"
	CdName      = "defang-cd"
)

type DoApp struct {
	Region     do.Region
	BucketName string
	AppID      string
}

const bucketPrefix = "defang-test" // FIXME: rename

func New(region do.Region) *DoApp {
	if region == "" {
		panic("region must be set")
	}

	return &DoApp{
		Region:     region,
		BucketName: os.Getenv("DEFANG_CD_BUCKET"),
	}
}

func (d *DoApp) GetBucketName(ctx context.Context, s3Client *s3.Client) (string, error) {
	bucketName := d.BucketName
	if bucketName == "" {
		lbo, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err != nil {
			return "", err
		}

		// Find an existing bucket that starts with the bucketPrefix
		for _, b := range lbo.Buckets {
			if strings.HasPrefix(*b.Name, bucketPrefix) {
				bucketName = *b.Name
				break
			}
		}
	}

	return bucketName, nil
}

func (d *DoApp) SetUpBucket(ctx context.Context) error {
	s3Client, err := d.CreateS3Client()
	if err != nil {
		return err
	}

	bucketName, err := d.GetBucketName(ctx, s3Client)
	if err != nil {
		return err
	}

	if bucketName == "" {
		bucketName = fmt.Sprintf("%s-%s", bucketPrefix, uuid.NewString())
		_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: &bucketName,
		})
	}

	d.BucketName = bucketName
	return err
}

func getImageSourceSpec(cdImagePath string) (*godo.ImageSourceSpec, error) {
	term.Debugf("Using CD image: %q", cdImagePath)
	image, err := ParseImage(cdImagePath)
	if err != nil {
		return nil, err
	}
	if image.Registry == "docker.io" || image.Registry == "index.docker.io" {
		image.Registry = path.Dir(image.Repo)
		image.Repo = path.Base(image.Repo)
	}
	if image.Digest != "" {
		// only one of jobs.image.tag or jobs.image.digest can be specified; digest takes precedence
		image.Tag = ""
	} else if image.Tag == "" {
		// default to tag "latest"
		image.Tag = "latest"
	}
	return &godo.ImageSourceSpec{
		// RegistryType: godo.ImageSourceSpecRegistryType_DOCR, TODO: support DOCR and GHCR
		// Repository:   "defangmvp/do-cd",
		RegistryType: godo.ImageSourceSpecRegistryType_DockerHub,
		Registry:     image.Registry,
		Repository:   image.Repo,
		Digest:       image.Digest,
		Tag:          image.Tag,
	}, nil
}

func (d DoApp) Run(ctx context.Context, env []*godo.AppVariableDefinition, cdImagePath string, cmd ...string) (*godo.App, error) {
	client := NewClient(ctx)

	image, err := getImageSourceSpec(cdImagePath)
	if err != nil {
		return nil, err
	}

	appJobSpec := &godo.AppSpec{
		Name:   CdName,
		Region: d.Region.String(),
		Jobs: []*godo.AppJobSpec{{
			Kind:             godo.AppJobSpecKind_PreDeploy,
			Name:             CdName,
			Envs:             env,
			Image:            image,
			InstanceCount:    1,
			InstanceSizeSlug: "basic-xs", // TODO: this is legacy and we should use new slugs
			RunCommand:       pkg.ShellQuote(cmd...),
			Termination: &godo.AppJobSpecTermination{
				GracePeriodSeconds: 600, // max 10mins to avoid killing the job while it's still running
			},
		}},
	}

	var currentCd = &godo.App{}

	appList, _, err := client.Apps.List(ctx, &godo.ListOptions{})
	if err != nil {
		term.Debugf("Error listing apps: %s", err)
	}

	for _, app := range appList {
		if app.Spec.Name == CdName {
			currentCd = app
		}
	}

	//Update current CD app if it exists
	if currentCd.Spec != nil && currentCd.Spec.Name != "" {
		term.Debugf("Updating existing CD app")
		currentCd, _, err = client.Apps.Update(ctx, currentCd.ID, &godo.AppUpdateRequest{
			Spec:                    appJobSpec,
			UpdateAllSourceVersions: true, // force update of the CD image
		})

		if err != nil {
			return nil, err
		}
	} else {
		term.Debugf("Creating new CD app")
		project, _, err := client.Projects.Create(ctx, &godo.CreateProjectRequest{
			Name:    CdName,
			Purpose: "Infrastructure for running Defang commands",
		})

		if err != nil {
			return nil, err
		}

		currentCd, _, err = client.Apps.Create(ctx, &godo.AppCreateRequest{
			Spec:      appJobSpec,
			ProjectID: project.ID,
		})

		if err != nil {
			return nil, err
		}
	}

	return currentCd, nil
}

// From https://github.com/digitalocean/doctl/blob/7fd3b7b253c7d6847b6b78d400eb26ed9be60796/commands/apps.go#L494
func waitForActiveDeployment(ctx context.Context, apps godo.AppsService, appID string, deploymentID string) error { // nolint: unused
	const maxAttempts = 180
	attempts := 0
	printNewLineSet := false

	for range make([]struct{}, maxAttempts) {
		if attempts != 0 {
			fmt.Fprint(os.Stderr, ".")
			if !printNewLineSet {
				printNewLineSet = true
				defer fmt.Fprintln(os.Stderr)
			}
		}

		deployment, _, err := apps.GetDeployment(ctx, appID, deploymentID)
		if err != nil {
			return err
		}

		allSuccessful := deployment.Progress.SuccessSteps == deployment.Progress.TotalSteps
		if allSuccessful {
			return nil
		}

		if deployment.Progress.ErrorSteps > 0 {
			return fmt.Errorf("error deploying app (%s) (deployment ID: %s):\n%s", appID, deployment.ID, godo.Stringify(deployment.Progress))
		}
		attempts++
		pkg.SleepWithContext(ctx, 10*time.Second) // was changed from time.Sleep
	}
	return fmt.Errorf("timeout waiting for app (%s) deployment", appID)
}

func NewClient(ctx context.Context) *godo.Client {
	accessToken := os.Getenv("DIGITALOCEAN_TOKEN")
	tokenSource := &oauth2.Token{AccessToken: accessToken}
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(tokenSource))
	return godo.NewClient(httpClient)
}

var s3InvalidCharsRegexp = regexp.MustCompile(`[^a-zA-Z0-9!_.*'()-]`)

func (d DoApp) CreateUploadURL(ctx context.Context, name string) (string, error) {
	s3Client, err := d.CreateS3Client()
	if err != nil {
		return "", err
	}

	prefix := "uploads/"

	if name == "" {
		name = uuid.NewString()
	} else {
		if len(name) > 64 {
			return "", errors.New("name must be less than 64 characters")
		}
		// Sanitize the digest so it's safe to use as a file name
		name = s3InvalidCharsRegexp.ReplaceAllString(name, "_")
		// name = path.Join(buildsPath, tenantName.String(), digest); TODO: avoid collisions between tenants
	}

	// Use S3 SDK to create a presigned URL for uploading a file.
	// FIXME: we need S3 auth anyway for Kaniko to be able to download the context from the bucket,
	// so should we just stick to the S3 SDK for all S3 operations, instead of using presigned URLs?
	req, err := s3.NewPresignClient(s3Client).PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: &d.BucketName,
		Key:    ptr.String(prefix + name),
	})

	if err != nil {
		return "", err
	}

	return req.URL, nil
}

func (d DoApp) CreateS3DownloadUrl(ctx context.Context, name string) (string, error) {
	s3Client, err := d.CreateS3Client()
	if err != nil {
		return "", err
	}

	req, err := s3.NewPresignClient(s3Client).PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &d.BucketName,
		Key:    &name,
	})
	if err != nil {
		return "", err
	}

	return req.URL, nil
}

func (d DoApp) CreateS3Client() (*s3.Client, error) {
	id := os.Getenv("SPACES_ACCESS_KEY_ID")
	key := os.Getenv("SPACES_SECRET_ACCESS_KEY")
	if id == "" || key == "" {
		return nil, errors.New("DigitalOcean SPACES_ACCESS_KEY_ID and SPACES_SECRET_ACCESS_KEY must be set")
	}

	cfg := aws.Config{
		Credentials:  credentials.NewStaticCredentialsProvider(id, key, ""),
		BaseEndpoint: ptr.String(fmt.Sprintf("https://%s.digitaloceanspaces.com", d.Region)),
		Region:       d.Region.String(),
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return s3Client, nil
}

// var _ clouds.Driver = (*DoAppPlatform)(nil)

// func (DoApp) GetInfo(context.Context, clouds.TaskID) (*clouds.TaskInfo, error) {
// 	panic("implement me")
// }

// func (DoApp) ListSecrets(context.Context) ([]string, error) {
// 	panic("implement me")
// }

// func (DoApp) PutSecret(context.Context, string, string) error {
// 	panic("implement me")
// }
