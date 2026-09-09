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

// mockRDSByArn simulates DescribeDBInstances(Filters:[db-instance-id=...]).
type mockRDSByArn struct {
	byArn map[string]rdstypes.DBInstance
}

func (m *mockRDSByArn) DescribeDBInstances(_ context.Context, in *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	var found []rdstypes.DBInstance
	for _, f := range in.Filters {
		if ptr.ToString(f.Name) != "db-instance-id" {
			continue
		}
		for _, arn := range f.Values {
			if inst, ok := m.byArn[arn]; ok {
				found = append(found, inst)
			}
		}
	}
	return &rds.DescribeDBInstancesOutput{DBInstances: found}, nil
}

func (m *mockRDSByArn) ModifyDBInstance(_ context.Context, _ *rds.ModifyDBInstanceInput, _ ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error) {
	return &rds.ModifyDBInstanceOutput{}, nil
}

func TestFindDBInstancesByArns(t *testing.T) {
	svc := &mockRDSByArn{byArn: map[string]rdstypes.DBInstance{
		"arn:a": {DBInstanceIdentifier: ptr.String("a")},
	}}
	found, err := FindDBInstancesByArns(t.Context(), []string{"arn:a", "arn:stale"}, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || *found[0].DBInstanceIdentifier != "a" {
		t.Fatalf("expected only the live instance, got %+v", found)
	}
}

func TestFindDBInstancesByArns_Empty(t *testing.T) {
	found, err := FindDBInstancesByArns(t.Context(), nil, &mockRDSByArn{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("expected no instances, got %+v", found)
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
