package authv1

// ClientIPMetadata is the gRPC metadata key in which the gateway passes
// the end user's address. Auth trusts it only from the gateway's mTLS
// identity. Written by hand, not generated.
const ClientIPMetadata = "x-client-ip"

// UserAgentMetadata is the key in which the gateway passes what the end
// user's browser calls itself, trusted as ClientIPMetadata is. Printable
// ASCII only: gRPC takes nothing else in metadata.
const UserAgentMetadata = "x-client-user-agent"
