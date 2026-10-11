package grpc

import authv1 "github.com/vexner67/freenet/auth/api/auth/v1"

// PublicMethods can be called without an access token: they are how one
// is got, and say who is calling by a code or a refresh token. Every
// other RPC requires an access token (see jwtauth.UnaryServerInterceptor).
func PublicMethods() []string {
	return []string{
		authv1.AuthService_RequestCode_FullMethodName,
		authv1.AuthService_VerifyCode_FullMethodName,
		authv1.AuthService_RefreshToken_FullMethodName,
		authv1.AuthService_Logout_FullMethodName,
	}
}
