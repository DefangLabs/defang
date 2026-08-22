package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go/ptr"
)

type RDSAPI interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	ModifyDBInstance(ctx context.Context, params *rds.ModifyDBInstanceInput, optFns ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error)
}

// FindDBInstancesByPrefix returns the DB instances whose identifier starts with prefix.
// Note: this covers standalone RDS instances; Aurora clusters are not handled here.
func FindDBInstancesByPrefix(ctx context.Context, prefix string, svc RDSAPI) ([]rdstypes.DBInstance, error) {
	var found []rdstypes.DBInstance
	var marker *string
	for {
		out, err := svc.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{Marker: marker})
		if err != nil {
			return nil, err
		}
		for _, inst := range out.DBInstances {
			if inst.DBInstanceIdentifier != nil && strings.HasPrefix(*inst.DBInstanceIdentifier, prefix) {
				found = append(found, inst)
			}
		}
		if out.Marker == nil {
			return found, nil
		}
		marker = out.Marker
	}
}

// SetDBInstanceDeletionProtection enables or disables deletion protection on the DB instance.
// Disabling it lets a subsequent `defang down` (Pulumi) delete the instance.
func SetDBInstanceDeletionProtection(ctx context.Context, id string, enabled bool, svc RDSAPI) error {
	_, err := svc.ModifyDBInstance(ctx, &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier: &id,
		DeletionProtection:   ptr.Bool(enabled),
		ApplyImmediately:     ptr.Bool(true),
	})
	return err
}
