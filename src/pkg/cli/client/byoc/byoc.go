package byoc

import (
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/cli/client/byoc/clouds"
)

var _ client.Client = (*clouds.ByocAws)(nil)
