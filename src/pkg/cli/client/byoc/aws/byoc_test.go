package aws

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go/ptr"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestDomainMultipleProjectSupport(t *testing.T) {
	port80 := &composeTypes.ServicePortConfig{Mode: "ingress", Target: 80}
	port8080 := &composeTypes.ServicePortConfig{Mode: "ingress", Target: 8080}
	hostModePort := &composeTypes.ServicePortConfig{Mode: "host", Target: 80}
	tests := []struct {
		ProjectName string
		TenantName  types.TenantName
		Fqn         string
		Port        *composeTypes.ServicePortConfig
		EndPoint    string
		PublicFqdn  string
		PrivateFqdn string
	}{
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "web", hostModePort, "web.project1.internal:80", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "api", port8080, "api--8080.project1.example.com", "api.project1.example.com", "api.project1.internal"},
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Tenant2", "tenant1", "web", port80, "web--80.tenant2.example.com", "web.tenant2.example.com", "web.tenant2.internal"},
		{"tenant1", "tenAnt1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
	}

	for _, tt := range tests {
		t.Run(tt.ProjectName+","+string(tt.TenantName), func(t *testing.T) {
			//like calling NewByocProvider(), but without needing real AccountInfo data
			b := &ByocAws{
				driver: cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
			}
			b.ByocBaseClient = byoc.NewByocBaseClient(context.Background(), tt.TenantName, b)

			const delegateDomain = "example.com"

			endpoint := b.getEndpoint(tt.Fqn, tt.ProjectName, delegateDomain, tt.Port)
			if endpoint != tt.EndPoint {
				t.Errorf("expected endpoint %q, got %q", tt.EndPoint, endpoint)
			}

			publicFqdn := b.getPublicFqdn(tt.ProjectName, delegateDomain, tt.Fqn)
			if publicFqdn != tt.PublicFqdn {
				t.Errorf("expected public fqdn %q, got %q", tt.PublicFqdn, publicFqdn)
			}

			privateFqdn := b.getPrivateFqdn(tt.ProjectName, tt.Fqn)
			if privateFqdn != tt.PrivateFqdn {
				t.Errorf("expected private fqdn %q, got %q", tt.PrivateFqdn, privateFqdn)
			}
		})
	}
}

type FakeLoader struct {
	ProjectName string
}

func (f FakeLoader) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return &composeTypes.Project{Name: f.ProjectName}, nil
}

func (f FakeLoader) LoadProjectName(ctx context.Context) (string, error) {
	return f.ProjectName, nil
}

//go:embed testdata/*.json
var testDir embed.FS

//go:embed testdata/*.events
var expectedDir embed.FS

func TestSubscribe(t *testing.T) {
	t.Skip("Pending test")
	tests, err := testDir.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to load ecs events test files: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.Name(), func(t *testing.T) {
			start := strings.LastIndex(tt.Name(), "-")
			end := strings.LastIndex(tt.Name(), ".")
			if start == -1 || end == -1 {
				t.Fatalf("cannot find etag from invalid test file name: %s", tt.Name())
			}
			name := tt.Name()[:start]
			etag := tt.Name()[start+1 : end]

			byoc := &ByocAws{}

			resp, err := byoc.Subscribe(context.Background(), &defangv1.SubscribeRequest{
				Etag:     etag,
				Services: []string{"api", "web"},
			})
			if err != nil {
				t.Fatalf("Subscribe() failed: %v", err)
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()

				filename := path.Join("testdata", name+".events")
				ef, _ := expectedDir.ReadFile(filename)
				dec := json.NewDecoder(bytes.NewReader(ef))

				for {
					if !resp.Receive() {
						if resp.Err() != nil {
							t.Errorf("Receive() failed: %v", resp.Err())
						}
						break
					}
					msg := resp.Msg()
					var expected defangv1.SubscribeResponse
					if err := dec.Decode(&expected); err == io.EOF {
						t.Errorf("unexpected message: %v", msg)
					} else if err != nil {
						t.Errorf("error unmarshaling expected ECS event: %v", err)
					} else if msg.Name != expected.Name || msg.Status != expected.Status || msg.State != expected.State {
						t.Errorf("expected message-, got+\n-%v\n+%v", &expected, msg)
					}
				}
			}()

			data, err := testDir.ReadFile(path.Join("testdata", tt.Name()))
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}
			lines := bufio.NewScanner(bytes.NewReader(data))
			for lines.Scan() {
				ecsEvt, err := ecs.ParseECSEvent([]byte(lines.Text()))
				if err != nil {
					t.Fatalf("error parsing ECS event: %v", err)
				}

				byoc.HandleECSEvent(ecsEvt)
			}
			resp.Close()

			wg.Wait()
		})
	}
}

type r53HostedZone struct {
	r53types.HostedZone
	r53types.DelegationSet // no ID => not reusable
}

type route53API interface {
	aws.Route53API
	DeleteHostedZone(ctx context.Context, params *route53.DeleteHostedZoneInput, optFns ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error)
}

type r53Mock struct {
	hostedZones    []r53HostedZone
	delegationSets []r53types.DelegationSet
}

func (r r53Mock) ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	var hostedZones []r53types.HostedZone
	for _, hz := range r.hostedZones {
		if params.DNSName != nil && *hz.Name < *params.DNSName { // assume ASCII order
			continue
		}
		hostedZones = append(hostedZones, hz.HostedZone)
		if params.MaxItems != nil && len(hostedZones) >= int(*params.MaxItems) {
			break
		}
	}
	return &route53.ListHostedZonesByNameOutput{
		HostedZones:  hostedZones,
		DNSName:      params.DNSName,
		MaxItems:     params.MaxItems,
		HostedZoneId: params.HostedZoneId,
	}, nil
}

func (r r53Mock) ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	for _, hz := range r.hostedZones {
		if *hz.HostedZone.Id != *params.HostedZoneId {
			continue
		}
		var recordSets []r53types.ResourceRecord
		if params.StartRecordType == r53types.RRTypeNs {
			// Copy the NS records from the hosted zone
			for _, ns := range hz.NameServers {
				recordSets = append(recordSets, r53types.ResourceRecord{Value: ptr.String(ns)})
			}
		}
		return &route53.ListResourceRecordSetsOutput{
			MaxItems: params.MaxItems,
			ResourceRecordSets: []r53types.ResourceRecordSet{
				{
					Name:            ptr.String(*hz.Name),
					Type:            params.StartRecordType,
					ResourceRecords: recordSets,
				},
			},
		}, nil
	}
	return nil, errors.New("hosted zone not found")
}

func (r *r53Mock) CreateReusableDelegationSet(ctx context.Context, params *route53.CreateReusableDelegationSetInput, optFns ...func(*route53.Options)) (*route53.CreateReusableDelegationSetOutput, error) {
	for _, ds := range r.delegationSets {
		if *ds.CallerReference == *params.CallerReference {
			return nil, &r53types.DelegationSetAlreadyCreated{}
		}
	}
	var delegationSet *r53types.DelegationSet
	if params.HostedZoneId != nil {
		for _, hz := range r.hostedZones {
			if strings.HasSuffix(*hz.HostedZone.Id, *params.HostedZoneId) {
				delegationSet = &hz.DelegationSet
				break
			}
		}
		if delegationSet == nil {
			return nil, &r53types.NoSuchHostedZone{}
		}
		if delegationSet.Id != nil {
			return nil, &r53types.DelegationSetAlreadyReusable{}
		}
		delegationSet.Id = ptr.String("/delegationset/N" + strings.ToUpper(pkg.RandomID()))
		delegationSet.CallerReference = params.CallerReference
	} else {
		delegationSet = &r53types.DelegationSet{
			CallerReference: params.CallerReference,
			Id:              ptr.String("/delegationset/N" + strings.ToUpper(pkg.RandomID())),
			NameServers:     []string{r.randNameServer(), r.randNameServer()},
		}
	}
	r.delegationSets = append(r.delegationSets, *delegationSet)
	return &route53.CreateReusableDelegationSetOutput{
		DelegationSet: delegationSet,
		Location:      ptr.String("https://route53.amazonaws.com/2013-04-01" + *delegationSet.Id),
	}, nil
}

func (r r53Mock) ListReusableDelegationSets(ctx context.Context, params *route53.ListReusableDelegationSetsInput, optFns ...func(*route53.Options)) (*route53.ListReusableDelegationSetsOutput, error) {
	return &route53.ListReusableDelegationSetsOutput{
		DelegationSets: r.delegationSets,
		Marker:         params.Marker,
		MaxItems:       params.MaxItems,
	}, nil
}

func (r53Mock) randNameServer() string {
	return pkg.RandomID() + ".example.com"
}

func (r r53Mock) GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, optFns ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error) {
	for _, hz := range r.hostedZones {
		if strings.HasSuffix(*hz.HostedZone.Id, *params.Id) {
			return &route53.GetHostedZoneOutput{
				HostedZone:    &hz.HostedZone,
				DelegationSet: &hz.DelegationSet,
			}, nil
		}
	}
	return nil, &r53types.NoSuchHostedZone{}
}

func (r r53Mock) DeleteHostedZone(ctx context.Context, params *route53.DeleteHostedZoneInput, optFns ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error) {
	return &route53.DeleteHostedZoneOutput{}, nil
}

func (r *r53Mock) CreateHostedZone(ctx context.Context, params *route53.CreateHostedZoneInput, optFns ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error) {
	hostedZone := r53types.HostedZone{
		Id:              ptr.String("/hostedzone/Z" + strings.ToUpper(pkg.RandomID())),
		CallerReference: params.CallerReference,
		Config:          params.HostedZoneConfig,
		Name:            params.Name,
	}
	var delegationSet *r53types.DelegationSet
	for _, ds := range r.delegationSets {
		if strings.HasSuffix(*ds.Id, *params.DelegationSetId) {
			delegationSet = &ds
			break
		}
	}
	if delegationSet == nil {
		delegationSet = &r53types.DelegationSet{
			NameServers: []string{r.randNameServer(), r.randNameServer()},
		}
	}
	r.hostedZones = append(r.hostedZones, r53HostedZone{
		HostedZone:    hostedZone,
		DelegationSet: *delegationSet,
	})
	return &route53.CreateHostedZoneOutput{
		DelegationSet: delegationSet,
		HostedZone:    &hostedZone,
		Location:      ptr.String("https://route53.amazonaws.com/2013-04-01" + *hostedZone.Id),
	}, nil
}

func TestPrepareDomainDelegationMocked(t *testing.T) {
	testPrepareDomainDelegationNew(t, &r53Mock{})
	testPrepareDomainDelegationLegacy(t, &r53Mock{})
}

func testPrepareDomainDelegationNew(t *testing.T, r53Client route53API) {
	const projectDomain = "byoc.example.internal"

	nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, r53Client)
	if err != nil {
		t.Fatal(err)
	}
	if len(nsServers) == 0 {
		t.Error("expected name servers")
	}
	if delegationSetId == "" {
		t.Fatal("expected delegation set id")
	}

	t.Run("reuse existing delegation set", func(t *testing.T) {
		nsServers2, delegationSetId2, err := prepareDomainDelegation(ctx, projectDomain, r53Client)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(nsServers, nsServers2) {
			t.Error("expected same name servers")
		}
		if delegationSetId != delegationSetId2 {
			t.Error("expected same delegation set id")
		}
	})

	t.Run("reuse existing hosted zone", func(t *testing.T) {
		// Now create the hosted zone like Pulumi would
		hz, err := r53Client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
			CallerReference:  ptr.String(projectDomain + " from testPrepareDomainDelegationNew " + pkg.RandomID()),
			Name:             ptr.String(projectDomain),
			DelegationSetId:  ptr.String(delegationSetId),
			HostedZoneConfig: &r53types.HostedZoneConfig{},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, err := r53Client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
				Id: hz.HostedZone.Id,
			})
			if err != nil {
				t.Error(err)
			}
		})

		nsServers2, delegationSetId2, err := prepareDomainDelegation(ctx, projectDomain, r53Client)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(nsServers, nsServers2) {
			t.Error("expected same name servers")
		}
		if delegationSetId != delegationSetId2 {
			t.Error("expected same delegation set id")
		}
	})
}

func testPrepareDomainDelegationLegacy(t *testing.T, r53Client route53API) {
	const projectDomain = "byoc-legacy.example.internal"

	ctx := context.Background()

	// "Create" the legacy hosted zone
	hz, err := r53Client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		CallerReference: ptr.String(projectDomain + " from testPrepareDomainDelegationLegacy " + pkg.RandomID()),
		Name:            ptr.String(projectDomain),
		HostedZoneConfig: &r53types.HostedZoneConfig{
			Comment: ptr.String(aws.CreateHostedZoneCommentLegacy),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := r53Client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
			Id: hz.HostedZone.Id,
		})
		if err != nil {
			t.Error(err)
		}
	})

	nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, r53Client)
	if err != nil {
		t.Fatal(err)
	}
	if len(nsServers) == 0 {
		t.Error("expected name servers")
	}
	if !slices.Equal(nsServers, hz.DelegationSet.NameServers) {
		t.Error("expected same name servers")
	}
	if delegationSetId != "" {
		t.Fatal("expected no delegation set id")
	}
}
