package crun

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/DefangLabs/defang/src/pkg/crun/docker"
)

type DriverOption func(clouds.Driver) error

func createDriver(reg Region, opts ...DriverOption) (clouds.Driver, error) {
	var driver clouds.Driver
	switch reg {
	case "docker", "local", "":
		driver = docker.New()
	case
		region.AFSouth1,     // "af-south-1"
		region.APEast1,      // "ap-east-1"
		region.APNortheast1, // "ap-northeast-1"
		region.APNortheast2, // "ap-northeast-2"
		region.APNortheast3, // "ap-northeast-3"
		region.APSouth1,     // "ap-south-1"
		region.APSouth2,     // "ap-south-2"
		region.APSoutheast1, // "ap-southeast-1"
		region.APSoutheast2, // "ap-southeast-2"
		region.APSoutheast3, // "ap-southeast-3"
		region.APSoutheast4, // "ap-southeast-4"
		region.CACentral,    // "ca-central-1"
		region.CNNorth1,     // "cn-north-1"
		region.CNNorthwest1, // "cn-northwest-1"
		region.EUCentral1,   // "eu-central-1"
		region.EUCentral2,   // "eu-central-2"
		region.EUNorth1,     // "eu-north-1"
		region.EUSouth1,     // "eu-south-1"
		region.EUSouth2,     // "eu-south-2"
		region.EUWest1,      // "eu-west-1"
		region.EUWest2,      // "eu-west-2"
		region.EUWest3,      // "eu-west-3"
		region.MECentral1,   // "me-central-1"
		region.MESouth1,     // "me-south-1"
		region.SAEast1,      // "sa-east-1"
		region.USGovEast1,   // "us-gov-east-1"
		region.USGovWest1,   // "us-gov-west-1"
		region.USEast1,      // "us-east-1"
		region.USEast2,      // "us-east-2"
		region.USWest1,      // "us-west-1"
		region.USWest2:      // "us-west-2"
		driver = cfn.New(stackName(pkg.GetCurrentUser()), reg)
	default:
		return nil, fmt.Errorf("unsupported region: %v", reg)
	}

	for _, opt := range opts {
		if err := opt(driver); err != nil {
			return nil, err
		}
	}

	return driver, nil
}

func stackName(stack string) string {
	if stack == "" {
		return clouds.ProjectName
	}
	return clouds.ProjectName + "-" + stack
}
