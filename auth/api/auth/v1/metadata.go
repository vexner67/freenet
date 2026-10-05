package authv1

// ClientIPMetadata is the gRPC metadata key in which the gateway passes
// the end user's address. Auth trusts it only from the gateway's mTLS
// identity. Written by hand, not generated.
const ClientIPMetadata = "x-client-ip"
