package cfn

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

const minDelay = 1 * time.Second

// update1s is a functional option for cloudformation.StackUpdateCompleteWaiter that sets the MinDelay to 1s
func update1s(o *cloudformation.StackUpdateCompleteWaiterOptions) {
	o.MinDelay = minDelay
}

// delete1s is a functional option for cloudformation.StackDeleteCompleteWaiter that sets the MinDelay to 1s
func delete1s(o *cloudformation.StackDeleteCompleteWaiterOptions) {
	o.MinDelay = minDelay
}

// create1s is a functional option for cloudformation.StackCreateCompleteWaiter that sets the MinDelay to 1s
func create1s(o *cloudformation.StackCreateCompleteWaiterOptions) {
	o.MinDelay = minDelay
}
