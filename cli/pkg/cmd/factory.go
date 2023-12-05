package cmd

import (
	"github.com/defang-io/defang/cli/pkg/aws/ecs/pulumi"
	"github.com/defang-io/defang/cli/pkg/docker"
	"github.com/defang-io/defang/cli/pkg/types"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
)

const (
	stack = "dev" // TODO: make configurable
)

func createDriver(color Color, region Region) types.Driver {
	switch region {
	case "":
		return docker.New()
	case
		aws.RegionAFSouth1,     // "af-south-1"
		aws.RegionAPEast1,      // "ap-east-1"
		aws.RegionAPNortheast1, // "ap-northeast-1"
		aws.RegionAPNortheast2, // "ap-northeast-2"
		aws.RegionAPNortheast3, // "ap-northeast-3"
		aws.RegionAPSouth1,     // "ap-south-1"
		aws.RegionAPSouth2,     // "ap-south-2"
		aws.RegionAPSoutheast1, // "ap-southeast-1"
		aws.RegionAPSoutheast2, // "ap-southeast-2"
		aws.RegionAPSoutheast3, // "ap-southeast-3"
		aws.RegionAPSoutheast4, // "ap-southeast-4"
		aws.RegionCACentral,    // "ca-central-1"
		aws.RegionCNNorth1,     // "cn-north-1"
		aws.RegionCNNorthwest1, // "cn-northwest-1"
		aws.RegionEUCentral1,   // "eu-central-1"
		aws.RegionEUCentral2,   // "eu-central-2"
		aws.RegionEUNorth1,     // "eu-north-1"
		aws.RegionEUSouth1,     // "eu-south-1"
		aws.RegionEUSouth2,     // "eu-south-2"
		aws.RegionEUWest1,      // "eu-west-1"
		aws.RegionEUWest2,      // "eu-west-2"
		aws.RegionEUWest3,      // "eu-west-3"
		aws.RegionMECentral1,   // "me-central-1"
		aws.RegionMESouth1,     // "me-south-1"
		aws.RegionSAEast1,      // "sa-east-1"
		aws.RegionUSGovEast1,   // "us-gov-east-1"
		aws.RegionUSGovWest1,   // "us-gov-west-1"
		aws.RegionUSEast1,      // "us-east-1"
		aws.RegionUSEast2,      // "us-east-2"
		aws.RegionUSWest1,      // "us-west-1"
		aws.RegionUSWest2:      // "us-west-2"
		return pulumi.New(stack, region, pulumi.Color(color))
	default:
		panic("unsupported region: " + region)
	}
}
