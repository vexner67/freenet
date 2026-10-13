// Package grpcclient connects freenet services to each other the same
// way: the request ID travels with every call, and a call made while the
// other service restarts waits for it instead of failing.
package grpcclient

import (
	"fmt"

	"github.com/vexner67/freenet/pkg/reqid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// waitForReady makes a call wait, within its deadline, for a connection
// that is down — a service restarted by a deployment is back in seconds —
// where it would fail at once. A call that has reached the service is
// never sent twice.
const waitForReady = `{"methodConfig": [{"name": [{}], "waitForReady": true}]}`

// New opens a connection to the service at addr; opts come last.
func New(addr string, creds credentials.TransportCredentials, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	own := append(reqid.DialOptions(),
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultServiceConfig(waitForReady),
	)

	conn, err := grpc.NewClient(addr, append(own, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("grpc.NewClient: %w", err)
	}

	return conn, nil
}
