package rest

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// maxBodyBytes is above any request of this API: a post of 60000
// characters, up to 12 bytes each when JSON escapes them (\ud83d\ude00).
const maxBodyBytes = 1 << 20

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return failure.New(failure.InvalidArgument, "request body is too large")
		}

		return failure.New(failure.InvalidArgument, "invalid JSON body: "+jsonProblem(err))
	}

	if dec.More() {
		return failure.New(failure.InvalidArgument, "invalid JSON body: unexpected data after the object")
	}

	return nil
}

// jsonProblem describes a decode error without echoing the input.
func jsonProblem(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return "field " + typeErr.Field + " has a wrong type"
	}

	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return strings.TrimPrefix(err.Error(), "json: ")
	}

	return "malformed"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorBody struct {
	Error errorDetails `json:"error"`
}

type errorDetails struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var kinds = map[failure.Kind]struct {
	status int
	code   string
}{
	failure.Internal:         {http.StatusInternalServerError, "internal"},
	failure.InvalidArgument:  {http.StatusBadRequest, "invalid_argument"},
	failure.Unauthenticated:  {http.StatusUnauthorized, "unauthenticated"},
	failure.PermissionDenied: {http.StatusForbidden, "permission_denied"},
	failure.NotFound:         {http.StatusNotFound, "not_found"},
	failure.Conflict:         {http.StatusConflict, "conflict"},
	failure.TooManyRequests:  {http.StatusTooManyRequests, "too_many_requests"},
	failure.Unavailable:      {http.StatusServiceUnavailable, "unavailable"},
	failure.MethodNotAllowed: {http.StatusMethodNotAllowed, "method_not_allowed"},
}

func (a *api) writeError(w http.ResponseWriter, r *http.Request, err error) {
	f := failure.From(err)

	switch f.Kind {
	case failure.Internal:
		a.logger.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
	case failure.Unavailable:
		a.logger.WarnContext(r.Context(), "dependency unavailable", "path", r.URL.Path, "error", err)
	}

	k := kinds[f.Kind]
	writeJSON(w, k.status, errorBody{Error: errorDetails{Code: k.code, Message: f.Message}})
}

// optionalBearerToken is "" without an Authorization header; a malformed
// header is still an error.
func optionalBearerToken(r *http.Request) (string, error) {
	if r.Header.Get("Authorization") == "" {
		return "", nil
	}

	return bearerToken(r)
}

// pageLimit reads ?limit=; 0 means the service default.
func pageLimit(r *http.Request) (int, error) {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > maxPageSize {
		return 0, failure.New(failure.InvalidArgument, "limit must be between 1 and 50")
	}

	return n, nil
}

func bearerToken(r *http.Request) (string, error) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", failure.New(failure.Unauthenticated, "missing access token")
	}

	return token, nil
}

// clientIP is the end user's address. X-Forwarded-For is honoured only when
// the gateway sits behind a proxy we control (TRUST_FORWARDED_FOR); then the
// rightmost entry is the one that proxy added, earlier ones can be forged.
func clientIP(r *http.Request, trustForwardedFor bool) string {
	if trustForwardedFor {
		if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
			hops := strings.Split(values[len(values)-1], ",")
			if addr, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1])); err == nil {
				return addr.String()
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}

	return host
}
