package store

import defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"

var (
	ReadOnlyUserWhoAmI bool = false
	UserWhoAmI         *defangv1.WhoAmIResponse
)

func SetUserWhoAmI(whoami *defangv1.WhoAmIResponse) {
	if !ReadOnlyUserWhoAmI {
		UserWhoAmI = whoami
	}
}
