package rest

// The security policies, for the tests of the external package.
const (
	PageCSP = pageCSP
	APICSP  = apiCSP
)

// Timeout is the middleware that bounds requests, and UploadReadTimeout
// how long the body of an upload may take.
var Timeout = timeout

const UploadReadTimeout = uploadReadTimeout
