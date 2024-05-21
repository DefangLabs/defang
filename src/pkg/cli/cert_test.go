package cli

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func TestWaitForCNAME(t *testing.T) {
	term.DoDebug = true
	ctx := context.Background()
	domain := "www.interviewprep.study"
	waitForCNAME(ctx, domain, "fabric-prod1.defang.dev")
}
