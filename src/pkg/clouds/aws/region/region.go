package region

import (
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
)

type Region = aws.Region

const (
	AFSouth1     Region = "af-south-1"
	APEast1      Region = "ap-east-1"
	APNortheast1 Region = "ap-northeast-1"
	APNortheast2 Region = "ap-northeast-2"
	APNortheast3 Region = "ap-northeast-3"
	APSouth1     Region = "ap-south-1"
	APSouth2     Region = "ap-south-2"
	APSoutheast1 Region = "ap-southeast-1"
	APSoutheast2 Region = "ap-southeast-2"
	APSoutheast3 Region = "ap-southeast-3"
	APSoutheast4 Region = "ap-southeast-4"
	CACentral    Region = "ca-central-1"
	CNNorth1     Region = "cn-north-1"
	CNNorthwest1 Region = "cn-northwest-1"
	EUCentral1   Region = "eu-central-1"
	EUCentral2   Region = "eu-central-2"
	EUNorth1     Region = "eu-north-1"
	EUSouth1     Region = "eu-south-1"
	EUSouth2     Region = "eu-south-2"
	EUWest1      Region = "eu-west-1"
	EUWest2      Region = "eu-west-2"
	EUWest3      Region = "eu-west-3"
	MECentral1   Region = "me-central-1"
	MESouth1     Region = "me-south-1"
	SAEast1      Region = "sa-east-1"
	USGovEast1   Region = "us-gov-east-1"
	USGovWest1   Region = "us-gov-west-1"
	USEast1      Region = "us-east-1"
	USEast2      Region = "us-east-2"
	USWest1      Region = "us-west-1"
	USWest2      Region = "us-west-2"
)

func FromArn(arn string) Region {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 || parts[0] != "arn" {
		panic("invalid ARN")
	}
	return Region(parts[3])
}

func Values() []Region {
	var zero Region
	return zero.Values()
}
