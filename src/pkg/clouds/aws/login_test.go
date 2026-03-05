package aws

import "testing"

func TestSameRole(t *testing.T) {
	tests := []struct {
		name    string
		a1      string
		a2      string
		want    bool
		wantErr bool
	}{
		{
			name: "IAM vs IAM same role",
			a1:   "arn:aws:iam::381492210770:role/admin",
			a2:   "arn:aws:iam::381492210770:role/admin",
			want: true,
		},
		{
			name: "STS vs IAM same role",
			a1:   "arn:aws:sts::381492210770:assumed-role/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/admin",
			want: true,
		},
		{
			name: "STS vs STS same role",
			a1:   "arn:aws:sts::381492210770:assumed-role/admin/session1",
			a2:   "arn:aws:sts::381492210770:assumed-role/admin/session2",
			want: true,
		},
		{
			name: "Different role names",
			a1:   "arn:aws:sts::381492210770:assumed-role/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/dev",
			want: false,
		},
		{
			name: "Different accounts",
			a1:   "arn:aws:sts::111111111111:assumed-role/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/admin",
			want: false,
		},
		{
			name: "Role path test",
			a1:   "arn:aws:sts::381492210770:assumed-role/team/dev/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/team/dev/admin",
			want: true,
		},
		{
			name:    "Malformed ARN",
			a1:      "not-an-arn",
			a2:      "arn:aws:iam::381492210770:role/admin",
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sameRole(tt.a1, tt.a2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SameRole() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("SameRole() = %v, want %v", got, tt.want)
			}
		})
	}
}
