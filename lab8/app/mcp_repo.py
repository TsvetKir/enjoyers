from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations
from mcp.server.transport_security import TransportSecuritySettings
from app.repository import release_section

mcp = FastMCP("parceldesk-repository", host="0.0.0.0", port=8001,
              stateless_http=False, json_response=True,
              transport_security=TransportSecuritySettings(
                  enable_dns_rebinding_protection=True,
                  allowed_hosts=["mcp-repo:8001", "localhost:8001", "127.0.0.1:8001"],
                  allowed_origins=["http://localhost:8001", "http://127.0.0.1:8001"]))
READ_ONLY = ToolAnnotations(readOnlyHint=True, destructiveHint=False, idempotentHint=True, openWorldHint=False)

@mcp.tool(annotations=READ_ONLY)
def get_release_changes(release: str) -> dict:
    """Read release commits, service, database migration and rollback readiness."""
    return release_section(release, "changes")

@mcp.tool(annotations=READ_ONLY)
def get_ci_status(release: str) -> dict:
    """Read CI check results and test coverage for a release."""
    return release_section(release, "ci")

if __name__ == "__main__":
    mcp.run(transport="streamable-http")
