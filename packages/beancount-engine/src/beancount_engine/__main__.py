"""One bounded JSON request on stdin and one response on stdout."""
import json
import sys
from .workspace import snapshot, report, read_file, mutate, history, documents, revision, WorkspaceError


def main():
    try:
        request = json.load(sys.stdin)
        op, path = request.get('operation'), request['path']
        args = request.get('args') or {}
        if op == 'snapshot': result = snapshot(path)
        elif op == 'report': result = report(path, args)
        elif op == 'revision': result = {'revision': revision(documents(path)[1])}
        elif op == 'read_file': result = read_file(path, args.get('path', ''))
        elif op == 'history': result = history(path)
        else: result = mutate(path, dict(args, operation=op))
        print(json.dumps(result))
    except WorkspaceError as error:
        print(json.dumps({'error': str(error), 'code': error.code}))
    except Exception as error:
        print(f'{type(error).__name__}: {error}', file=sys.stderr)
        raise SystemExit(1)


if __name__ == '__main__':
    main()
