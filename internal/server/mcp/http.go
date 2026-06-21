package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewHTTPHandler wraps the MCP server in a Streamable HTTP handler protected by
// OAuth 2.1 bearer-token auth. Unauthenticated requests get a 401 with a
// WWW-Authenticate header pointing at the protected-resource metadata, per
// RFC 9728 — the MCP authorization handshake. The verifier (supplied by the
// auth package) validates the token and yields the user id.
func NewHTTPHandler(srv *mcp.Server, verifier auth.TokenVerifier, resourceMetadataURL string) http.Handler {
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	return auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: resourceMetadataURL,
	})(streamable)
}
