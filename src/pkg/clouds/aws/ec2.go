package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
)

func (a *Aws) RevokeDefaultSecurityGroupRules(ctx context.Context, sgId string) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	var errs []error
	ec2Client := ec2.NewFromConfig(cfg)
	_, err = ec2Client.RevokeSecurityGroupEgress(ctx, &ec2.RevokeSecurityGroupEgressInput{
		GroupId: ptr.String(sgId),
		IpPermissions: []types.IpPermission{{
			IpProtocol: ptr.String("-1"),
			FromPort:   ptr.Int32(-1),
			ToPort:     ptr.Int32(-1),
			IpRanges: []types.IpRange{{
				CidrIp: ptr.String("0.0.0.0/0"),
			}},
		}},
	})
	if err != nil && !isInvalidPermissionNotFoundErr(err) {
		errs = append(errs, err)
	}

	_, err = ec2Client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
		GroupId: ptr.String(sgId),
		IpPermissions: []types.IpPermission{{
			IpProtocol: ptr.String("-1"),
			FromPort:   ptr.Int32(-1),
			ToPort:     ptr.Int32(-1),
			UserIdGroupPairs: []types.UserIdGroupPair{{
				GroupId: ptr.String(sgId), // default SG allows all inbound from itself
			}},
		}},
	})
	if err != nil && !isInvalidPermissionNotFoundErr(err) {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func isInvalidPermissionNotFoundErr(err error) bool {
	// If the values you specify do not match the existing rule's values, an
	// InvalidPermission.NotFound client error is returned, and no rules are revoked.
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.NotFound"
}
