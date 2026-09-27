"""Check that a deployed OIDC service fails closed without credentials."""
import json
import sys
import time
import urllib.error
import urllib.request

base = sys.argv[1].rstrip('/')

def request(path, data=None):
    req = urllib.request.Request(base + path, data=data,
        headers={'Accept': 'application/json, text/event-stream', 'Content-Type': 'application/json', 'Origin': base, 'Connect-Protocol-Version': '1'})
    try:
        return urllib.request.urlopen(req, timeout=10)
    except urllib.error.HTTPError as error:
        return error

for attempt in range(60):
    try:
        with request('/beancount.v1.LedgerService/GetSession', b'{}') as response:
            state = json.load(response)
            if response.status == 200 and state.get('authEnabled', False) and not state.get('authenticated', False):
                break
    except (ValueError, KeyError, urllib.error.URLError, TimeoutError):
        pass
    time.sleep(2)
else:
    raise SystemExit('OIDC deployment did not become ready')

with request('/mcp', b'{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}') as response:
    assert response.status == 401, 'MCP accepted an unauthenticated request'
    assert 'resource_metadata=' in response.headers['WWW-Authenticate']
with request('/beancount.v1.LedgerService/GetSnapshot', b'{}') as response:
    assert response.status == 401, 'Identity API accepted an unauthenticated request'
with request('/.well-known/oauth-protected-resource/mcp') as response:
    metadata = json.load(response)
    assert metadata['resource'] == base + '/mcp'
    assert metadata['authorization_servers']
print('OIDC state, protected MCP/API, and OAuth discovery passed')
