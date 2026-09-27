"""Check the UI and MCP against a running sample-ledger server."""

import json
import sys
import time
import urllib.error
import urllib.request

base = sys.argv[1].rstrip("/")
for attempt in range(60):
    try:
        with urllib.request.urlopen(base + "/healthz", timeout=1) as response:
            assert json.load(response)["status"] == "ok"
        break
    except (urllib.error.URLError, ConnectionError, TimeoutError):
        time.sleep(0.5)
else:
    raise RuntimeError("container did not become ready")
request = urllib.request.Request(base + "/journal", headers={"Accept": "text/html"})
with urllib.request.urlopen(request, timeout=5) as response:
    assert response.status == 200 and b"<html" in response.read()
session_id = None
def rpc(method, params):
    global session_id
    request = urllib.request.Request(base + "/mcp",
        data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode(),
        headers={"Content-Type": "application/json", "Accept": "application/json, text/event-stream",
                 "MCP-Protocol-Version": "2025-06-18", **({"Mcp-Session-Id": session_id} if session_id else {})})
    with urllib.request.urlopen(request, timeout=10) as response:
        session_id = response.headers.get("Mcp-Session-Id", session_id)
        return json.load(response)["result"]
assert rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
           "clientInfo": {"name": "smoke", "version": "1"}})["serverInfo"]["name"] == "beanframe"
assert len(rpc("tools/list", {})["tools"]) == 14
assert rpc("tools/call", {"name": "validate_ledger", "arguments": {}})["structuredContent"]["valid"]
def tool(name, **arguments):
    result = rpc("tools/call", {"name": name, "arguments": arguments})
    assert not result.get("isError"), result
    return result["structuredContent"]

snapshot = tool("get_ledger_snapshot")
created = tool("save_transaction", expected_revision=snapshot["revision"],
    source='2026-01-10 * "Container smoke write"\n  Expenses:Hosting  1.00 USD\n  Assets:Checking  -1.00 USD\n')
snapshot = tool("get_ledger_snapshot")
transaction = next(tx for tx in snapshot["transactions"] if tx["narration"] == "Container smoke write")
tool("delete_transaction", expected_revision=created["revision"], id=transaction["id"])
assert tool("validate_ledger")["valid"]
request = urllib.request.Request(base + "/beancount.v1.LedgerService/GetSession",
    data=b"{}", headers={"Content-Type": "application/json", "Connect-Protocol-Version": "1"})
with urllib.request.urlopen(request, timeout=5) as response:
    assert json.load(response)["canWrite"]
print("UI, ConnectRPC, and MCP reads/writes passed on the same listener")
