package actions

import (
	"context"
	"errors"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
)

// Check if the provided AWS access key ID is valid
// https://medium.com/@TalBeerySec/a-short-note-on-aws-key-id-f88cc4317489
func IsValidAWSKey(key string) bool {
	// Define accepted AWS access key prefixes
	acceptedPrefixes := map[string]bool{
		"ABIA": true,
		"ACCA": true,
		"AGPA": true,
		"AIDA": true,
		"AKPA": true,
		"AKIA": true,
		"ANPA": true,
		"ANVA": true,
		"APKA": true,
		"AROA": true,
		"ASCA": true,
		"ASIA": true,
	}

	if len(key) < 16 {
		return false
	}

	prefix := key[:4]
	_, ok := acceptedPrefixes[prefix]
	if !ok {
		return false
	}

	return true
}

func SetAWSByocProvider(ctx context.Context, providerId *client.ProviderID, cluster string, accessKeyId string, secretKey string, region string) error {
	// Can never be nil or empty due to RequiredArgument
	if IsValidAWSKey(accessKeyId) {
		err := os.Setenv("AWS_ACCESS_KEY_ID", accessKeyId)
		if err != nil {
			return err
		}

		if secretKey == "" {
			return errors.New("AWS_SECRET_ACCESS_KEY is required")
		}

		err = os.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
		if err != nil {
			return err
		}

		if region == "" {
			return errors.New("AWS_REGION is required")
		}

		err = os.Setenv("AWS_REGION", region)
		if err != nil {
			return err
		}
	} else {
		err := os.Setenv("AWS_PROFILE", accessKeyId)
		if err != nil {
			return err
		}

		if region != "" {
			err = os.Setenv("AWS_REGION", region)
			if err != nil {
				return err
			}
		}
	}

	fabric, err := common.Connect(ctx, cluster)
	if err != nil {
		return err
	}

	_, err = common.CheckProviderConfigured(ctx, fabric, client.ProviderAWS, "", 0)
	if err != nil {
		return err
	}

	*providerId = client.ProviderAWS

	//FIXME: Should not be setting both the global and env var
	err = os.Setenv("DEFANG_PROVIDER", "aws")
	if err != nil {
		return err
	}

	return nil
}
