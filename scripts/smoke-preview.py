"""Validate the large preview seed through the real ConnectRPC API."""
import json
import sys
import time
import urllib.error
import urllib.request

base=sys.argv[1].rstrip('/')
def rpc(method, data):
    req=urllib.request.Request(base+'/beancount.v1.LedgerService/'+method,
        data=json.dumps(data).encode(),headers={'Content-Type':'application/json','Connect-Protocol-Version':'1'})
    with urllib.request.urlopen(req,timeout=20) as response:
        return json.load(response)

for _ in range(60):
    try:
        snapshot=rpc('GetSnapshot',{})
        break
    except (urllib.error.URLError,TimeoutError,ConnectionError):
        time.sleep(0.5)
else:
    raise SystemExit('Preview seed did not become ready')
assert len(snapshot['transactions'])==1491
assert len(snapshot['accounts'])==70
assert not snapshot.get('errors')
report=rpc('GetReport',{'currency':'USD'})
assert len(report['periods'])==45
file=rpc('ReadFile',{'path':'main.beancount'})
changed=rpc('WriteFile',{'path':file['path'],'content':file['content']+'\n','expectedRevision':file['revision']})
assert changed['revision']!=file['revision']
assert len(rpc('GetSnapshot',{})['transactions'])==1491
assert rpc('GetHistory',{})['entries']
print('Large preview seed: 1,491 transactions, 70 accounts, 45 months; validated write and history passed')
