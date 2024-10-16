package appPlatform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
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
	CDName = "defang-cd"
)

var (
	// Changing this will cause issues if two clients with different versions are using the same account
	CdImage = pkg.Getenv("DEFANG_CD_IMAGE", "defangio/cd:"+byoc.CdImageTag)
)

type DoApp struct {
	Region      do.Region
	ProjectName string
	BucketName  string
	AppID       string
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

func (d *DoApp) SetUp(ctx context.Context) error {
	s3Client, err := d.createS3Client()
	if err != nil {
		return err
	}

	lbo, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return err
	}

	if d.BucketName == "" {
		// Find an existing bucket that starts with the bucketPrefix
		for _, b := range lbo.Buckets {
			if strings.HasPrefix(*b.Name, bucketPrefix) {
				d.BucketName = *b.Name
				break
			}
		}
	}

	if d.BucketName == "" {
		d.BucketName = fmt.Sprintf("%s-%s", bucketPrefix, uuid.NewString())
		_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: &d.BucketName,
		})
	}

	return err
}

func shellQuote(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = strconv.Quote(arg)
	}
	return strings.Join(quoted, " ")
}

func getCdImage() (*godo.ImageSourceSpec, error) {
	image, err := ParseImage(CdImage)
	if err != nil {
		return nil, err
	}
	if image.Registry == "docker.io" || image.Registry == "index.docker.io" {
		image.Registry = path.Dir(image.Repo)
		image.Repo = path.Base(image.Repo)
	}
	if image.Tag == "" && image.Digest == "" {
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

func (d DoApp) Run(ctx context.Context, env []*godo.AppVariableDefinition, cmd ...string) (*godo.App, error) {
	client, err := NewClient(ctx)
	if err != nil {
		return nil, err
	}

	image, err := getCdImage()
	if err != nil {
		return nil, err
	}

	appJobSpec := &godo.AppSpec{
		Name:   CDName,
		Region: d.Region.String(),
		Jobs: []*godo.AppJobSpec{{
			Kind:             godo.AppJobSpecKind_PreDeploy,
			Name:             CDName,
			Envs:             env,
			Image:            image,
			InstanceCount:    1,
			InstanceSizeSlug: "basic-xs", // TODO: this is legacy and we should use new slugs
			RunCommand:       shellQuote(cmd...),
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
		if app.Spec.Name == CDName {
			currentCd = app
		}
	}

	//Update current CD app if it exists
	if currentCd.Spec != nil && currentCd.Spec.Name != "" {
		term.Debugf("Updating existing CD app")
		currentCd, _, err = client.Apps.Update(ctx, currentCd.ID, &godo.AppUpdateRequest{
			Spec: appJobSpec,
		})
	} else {
		term.Debugf("Creating new CD app")
		project, _, err := client.Projects.Create(ctx, &godo.CreateProjectRequest{
			Name:    CDName,
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
func waitForActiveDeployment(ctx context.Context, apps godo.AppsService, appID string, deploymentID string) error {
	const maxAttempts = 180
	attempts := 0
	printNewLineSet := false

	for i := 0; i < maxAttempts; i++ {
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
	return fmt.Errorf("timeout waiting to app (%s) deployment", appID)
}

func NewClient(ctx context.Context) (*godo.Client, error) {
	accessToken := os.Getenv("DIGITALOCEAN_TOKEN")
	if accessToken == "" {
		return nil, errors.New("DIGITALOCEAN_TOKEN must be set")
	}
	tokenSource := &oauth2.Token{AccessToken: accessToken}
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(tokenSource))
	return godo.NewClient(httpClient), nil
}

var s3InvalidCharsRegexp = regexp.MustCompile(`[^a-zA-Z0-9!_.*'()-]`)

func (d DoApp) CreateUploadURL(ctx context.Context, name string) (string, error) {
	s3Client, err := d.createS3Client()
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
		// name = path.Join(buildsPath, tenantId.String(), digest); TODO: avoid collisions between tenants
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
	s3Client, err := d.createS3Client()
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

func (d DoApp) createS3Client() (*s3.Client, error) {
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

// var _ types.Driver = (*DoAppPlatform)(nil)

// func (DoApp) GetInfo(context.Context, types.TaskID) (*types.TaskInfo, error) {
// 	panic("implement me")
// }

// func (DoApp) ListSecrets(context.Context) ([]string, error) {
// 	panic("implement me")
// }

// func (DoApp) PutSecret(context.Context, string, string) error {
// 	panic("implement me")
// }
