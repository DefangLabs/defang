package store

import defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"

var (
	UserWhoAmI *defangv1.WhoAmIResponse
)

func SetUserWhoAmI(whoami *defangv1.WhoAmIResponse) {
	UserWhoAmI = whoami
}
