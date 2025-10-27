package rc

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load(".defangrc"); err != nil {
		term.Debugf("could not load .defangrc: %v", err)
	}
}
