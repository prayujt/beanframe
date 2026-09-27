"""Prepare branch-specific Nimbus config and resolve a preview's canonical origin."""
import json
import os
from pathlib import Path
import re
import urllib.error
import urllib.parse
import urllib.request
import uuid

BASE = os.environ.get('NIMBUS_URL', 'https://nimbus.prayujt.com').rstrip('/')
PROJECT = os.environ.get('NIMBUS_PROJECT', 'beanframe')
PUBLIC_URL = os.environ.get('PUBLIC_URL', 'https://beanframe.prayujt.com').rstrip('/')
PREVIEW_DOMAIN = os.environ.get('NIMBUS_PREVIEW_DOMAIN', 'prayujt.com')


def request(path, data=None, method=None, content_type='application/json'):
    req = urllib.request.Request(BASE + path, data=data, method=method,
        headers={'X-Api-Key': os.environ['NIMBUS_API_KEY'], 'Content-Type': content_type})
    with urllib.request.urlopen(req, timeout=180) as response:
        raw = response.read()
        return json.loads(raw) if raw else None


def preview_manifest(origin):
    source = Path('nimbus-preview.yaml').read_text()
    assert source.count('${PREVIEW_ORIGIN}') == 1
    return source.replace('${PREVIEW_ORIGIN}', origin)


def bootstrap(branch):
    # The first deployment allocates the URL. Its temporary origin fails closed:
    # the server rejects requests for the preview host until final deployment.
    boundary = uuid.uuid4().hex
    fields = [('file', preview_manifest('https://preview.invalid')),
              ('branch', branch), ('commit', os.environ['GITHUB_SHA'])]
    parts = []
    for name, value in fields:
        filename = '; filename="nimbus.yaml"' if name == 'file' else ''
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"{filename}\r\n\r\n{value}\r\n')
    payload = (''.join(parts) + f'--{boundary}--\r\n').encode()
    request('/deploy', payload, 'POST', f'multipart/form-data; boundary={boundary}')


def main():
    branch = os.environ['BRANCH_NAME']
    # Nimbus aliases master to the main namespace; never treat it as a preview.
    if not branch or branch == 'master':
        raise SystemExit('Refusing an empty branch or the reserved main-namespace alias master')
    query = urllib.parse.urlencode({'project': PROJECT, 'branch': branch})
    if os.environ.get('CLEANUP_BRANCH') == 'true':
        if branch == 'main':
            raise SystemExit('Refusing to delete production')
        try:
            request('/branch?' + query, method='DELETE')
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
        print('Preview resources and PVC cleanup requested')
        return
    if branch == 'main':
        source = Path('nimbus.yaml').read_text()
        origin = PUBLIC_URL
    else:
        try:
            service = request('/services/server?' + query)
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
            bootstrap(branch)
            service = request('/services/server?' + query)
        host = service['ingress']
        if not isinstance(host, str) or not re.fullmatch(r'[a-f0-9]{16}\.' + re.escape(PREVIEW_DOMAIN), host):
            raise SystemExit('Nimbus returned an unexpected preview hostname')
        origin = 'https://' + host
        source = preview_manifest(origin)
    Path(os.environ.get('DEPLOY_CONFIG', '.nimbus-deploy.yaml')).write_text(source)
    with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
        output.write(f'origin={origin}\n')
    print('Prepared ' + origin)


if __name__ == '__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        raise SystemExit(f'Nimbus request failed (HTTP {error.code})') from None
