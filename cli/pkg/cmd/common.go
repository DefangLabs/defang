package cmd

import (
	"fmt"
	"os"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
)

type Region = aws.Region

func Fatal(msg any) {
	fmt.Println("Error:", msg) // TODO: color red
	os.Exit(1)
}
