package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go/ptr"
)

type mockRDS struct {
	instances   []rdstypes.DBInstance
	modifyInput *rds.ModifyDBInstanceInput
}

func (m *mockRDS) DescribeDBInstances(_ context.Context, _ *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return &rds.DescribeDBInstancesOutput{DBInstances: m.instances}, nil
}

func (m *mockRDS) ModifyDBInstance(_ context.Context, in *rds.ModifyDBInstanceInput, _ ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error) {
	m.modifyInput = in
	return &rds.ModifyDBInstanceOutput{}, nil
}

func TestFindDBInstancesByPrefix(t *testing.T) {
	svc := &mockRDS{instances: []rdstypes.DBInstance{
		{DBInstanceIdentifier: ptr.String("defang-app-beta-db")},
		{DBInstanceIdentifier: ptr.String("unrelated-db")},
	}}
	found, err := FindDBInstancesByPrefix(t.Context(), "defang-app-beta", svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || *found[0].DBInstanceIdentifier != "defang-app-beta-db" {
		t.Fatalf("expected only the matching instance, got %+v", found)
	}
}

func TestSetDBInstanceDeletionProtection(t *testing.T) {
	svc := &mockRDS{}
	if err := SetDBInstanceDeletionProtection(t.Context(), "db", false, svc); err != nil {
		t.Fatal(err)
	}
	if svc.modifyInput == nil || *svc.modifyInput.DeletionProtection {
		t.Fatalf("expected deletion protection disabled, got %+v", svc.modifyInput)
	}
}
