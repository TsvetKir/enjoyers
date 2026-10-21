from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations
from mcp.server.transport_security import TransportSecuritySettings
from app.repository import policy, release_section

mcp = FastMCP("parceldesk-knowledge", host="0.0.0.0", port=8002,
              stateless_http=False, json_response=True,
              transport_security=TransportSecuritySettings(
                  enable_dns_rebinding_protection=True,
                  allowed_hosts=["mcp-knowledge:8002", "localhost:8002", "127.0.0.1:8002"],
                  allowed_origins=["http://localhost:8002", "http://127.0.0.1:8002"]))
READ_ONLY = ToolAnnotations(readOnlyHint=True, destructiveHint=False, idempotentHint=True, openWorldHint=False)

@mcp.tool(annotations=READ_ONLY)
def get_release_policy(service: str) -> dict:
    """Read required CI checks, minimum coverage and blocking risk policy for a service."""
    return policy(service)

@mcp.tool(annotations=READ_ONLY)
def get_known_risks(release: str) -> dict:
    """Read known risks with severity and resolution status for a release."""
    return release_section(release, "risks")

if __name__ == "__main__":
    mcp.run(transport="streamable-http")
