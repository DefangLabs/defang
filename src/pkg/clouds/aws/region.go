package aws

import (
	"strings"
)

type Region string

func (r Region) String() string {
	return string(r)
}

const (
	RegionAFSouth1     Region = "af-south-1"
	RegionAPEast1      Region = "ap-east-1"
	RegionAPNortheast1 Region = "ap-northeast-1"
	RegionAPNortheast2 Region = "ap-northeast-2"
	RegionAPNortheast3 Region = "ap-northeast-3"
	RegionAPSouth1     Region = "ap-south-1"
	RegionAPSouth2     Region = "ap-south-2"
	RegionAPSoutheast1 Region = "ap-southeast-1"
	RegionAPSoutheast2 Region = "ap-southeast-2"
	RegionAPSoutheast3 Region = "ap-southeast-3"
	RegionAPSoutheast4 Region = "ap-southeast-4"
	RegionCACentral    Region = "ca-central-1"
	RegionCNNorth1     Region = "cn-north-1"
	RegionCNNorthwest1 Region = "cn-northwest-1"
	RegionEUCentral1   Region = "eu-central-1"
	RegionEUCentral2   Region = "eu-central-2"
	RegionEUNorth1     Region = "eu-north-1"
	RegionEUSouth1     Region = "eu-south-1"
	RegionEUSouth2     Region = "eu-south-2"
	RegionEUWest1      Region = "eu-west-1"
	RegionEUWest2      Region = "eu-west-2"
	RegionEUWest3      Region = "eu-west-3"
	RegionMECentral1   Region = "me-central-1"
	RegionMESouth1     Region = "me-south-1"
	RegionSAEast1      Region = "sa-east-1"
	RegionUSGovEast1   Region = "us-gov-east-1"
	RegionUSGovWest1   Region = "us-gov-west-1"
	RegionUSEast1      Region = "us-east-1"
	RegionUSEast2      Region = "us-east-2"
	RegionUSWest1      Region = "us-west-1"
	RegionUSWest2      Region = "us-west-2"
)

func RegionFromArn(arn string) Region {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" {
		panic("invalid ARN")
	}
	return Region(parts[3])
}
