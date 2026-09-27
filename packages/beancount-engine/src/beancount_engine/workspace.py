"""Ledger snapshots and validated, optimistic, atomic single-file mutations."""
import contextlib
import datetime as dt
from decimal import Decimal
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import sys
import tempfile
import uuid

from beancount import loader
from beancount.core import convert, data, inventory
from beancount.parser import parser

MAX_BYTES = 16 * 1024 * 1024

class WorkspaceError(ValueError):
    def __init__(self, message, code="invalid_argument"):
        super().__init__(message)
        self.code = code


def documents(path):
    root = Path(path).resolve(strict=True).parent
    docs = {}
    for p in sorted(root.rglob('*.beancount')):
        rel = p.relative_to(root)
        if any(part.startswith('.') for part in rel.parts):
            continue
        if p.is_symlink() or not p.is_file() or not p.resolve().is_relative_to(root):
            raise WorkspaceError('Ledger files must be regular files within the ledger directory')
        docs[str(rel)] = p.read_bytes()
        if len(docs) > 512 or sum(map(len, docs.values())) > MAX_BYTES:
            raise WorkspaceError('Ledger exceeds the 512-file / 16 MiB workspace limit')
    return root, docs


def revision(docs):
    h = hashlib.sha256()
    for name, content in sorted(docs.items()):
        h.update(name.encode() + b'\0' + content + b'\0')
    return h.hexdigest()


def load(path):
    with tempfile.TemporaryDirectory(prefix='beancount-engine-') as cache:
        loader.initialize(True, str(Path(cache) / 'ledger.pickle'))
        with contextlib.redirect_stdout(sys.stderr):
            return loader.load_file(str(Path(path).resolve(strict=True)))


def amount(value):
    if value is None or not isinstance(value.number, Decimal):
        return None
    return {'number': str(value.number), 'currency': value.currency}


def location(entry, root):
    meta = entry.meta or {}
    filename = Path(meta.get('filename', root / 'unknown'))
    try:
        name = str(filename.resolve().relative_to(root))
    except ValueError:
        name = filename.name
    return name, meta.get('lineno', 0)


def span(content, line):
    lines = content.splitlines(keepends=True)
    start = line - 1
    end = start + 1
    while end < len(lines) and (not lines[end].strip() or lines[end][0].isspace()):
        end += 1
    while end > start + 1 and not lines[end-1].strip():
        end -= 1
    return lines, start, end


def snapshot(path):
    for _ in range(3):
        root, docs = documents(path)
        entries, errors, options = load(path)
        _, after = documents(path)
        if docs == after:
            break
    else:
        raise WorkspaceError('Ledger changed while reading; retry', 'aborted')
    for included in options.get('include', []):
        if not Path(included).resolve().is_relative_to(root):
            raise WorkspaceError('Includes must stay inside the ledger directory')
    accounts, transactions, balances = [], [], {}
    closes = {e.account: str(e.date) for e in entries if isinstance(e, data.Close)}
    for entry in entries:
        if isinstance(entry, data.Open):
            accounts.append({'name': entry.account, 'opened': str(entry.date),
                             'currencies': entry.currencies or [], 'closed': closes.get(entry.account, '')})
        elif isinstance(entry, data.Transaction):
            name, line = location(entry, root)
            source = ''
            if name in docs and line > 0:
                lines, start, end = span(docs[name].decode(), line)
                source = ''.join(lines[start:end]).rstrip()
            identifier = hashlib.sha256(f'{name}:{line}:{source}'.encode()).hexdigest()[:24]
            postings = []
            for posting in entry.postings:
                book = None
                if posting.units is not None and isinstance(posting.units.number, Decimal):
                    book = amount(convert.get_cost(posting))
                postings.append({'account': posting.account, 'units': amount(posting.units),
                                 'cost': str(posting.cost) if posting.cost else None,
                                 'price': amount(posting.price), 'book_value': book})
                if posting.units is not None and not errors:
                    balances.setdefault(posting.account, inventory.Inventory()).add_position(posting)
            transactions.append({'id': identifier, 'date': str(entry.date), 'flag': entry.flag,
                                 'payee': entry.payee or '', 'narration': entry.narration,
                                 'tags': sorted(entry.tags or []), 'links': sorted(entry.links or []),
                                 'postings': postings, 'file': name, 'line': line, 'source': source})
    diagnostics = []
    for error in errors:
        meta = error.source or {}
        filename = Path(meta.get('filename', root / 'unknown'))
        try:
            filename = filename.resolve().relative_to(root)
        except ValueError:
            filename = Path(filename.name)
        diagnostics.append({'message': error.message, 'file': str(filename), 'line': meta.get('lineno', 0)})
    currencies = sorted({p['units']['currency'] for t in transactions for p in t['postings'] if p['units']}
                        | {c for a in accounts for c in a['currencies']} | set(options.get('operating_currency', [])))
    return {'title': options.get('title', 'Beancount'), 'revision': revision(docs),
            'accounts': accounts, 'transactions': transactions,
            'balances': [{'account': name, 'positions': [
                {'units': amount(pos.units), 'cost': str(pos.cost) if pos.cost else None}
                for pos in balance.get_positions()]} for name, balance in sorted(balances.items())],
            'errors': [e['message'] for e in diagnostics], 'diagnostics': diagnostics,
            'files': [{'path': name, 'bytes': len(content)} for name, content in docs.items()],
            'currencies': currencies, 'operating_currencies': options.get('operating_currency', [])}


def security_lines(text):
    return [line.strip() for line in text.splitlines()
            if re.match(r'^\s*(include|plugin)\s|^\s*option\s+"insert_pythonpath"', line)]


def valid_date(value):
    try:
        if dt.date.fromisoformat(value).isoformat() != value:
            raise ValueError()
    except (TypeError, ValueError):
        raise WorkspaceError('Use an ISO date in YYYY-MM-DD format') from None
    return value


def require_account(name):
    if not re.fullmatch(r'(Assets|Liabilities|Equity|Income|Expenses)(:[A-Z][A-Za-z0-9-]*)+', name):
        raise WorkspaceError('Account must use a standard root and capitalized colon-separated components')


def history(path):
    root, _ = documents(path)
    directory = root / '.beancount-history'
    items = []
    if directory.is_symlink():
        raise WorkspaceError('Invalid history directory')
    for p in sorted(directory.glob('*.json'), reverse=True):
        if p.is_symlink():
            continue
        item = json.loads(p.read_text())
        items.append({k: v for k, v in item.items() if k != 'before_content'})
    return {'entries': items[:200]}


def read_file(path, name):
    _, docs = documents(path)
    if name not in docs:
        raise WorkspaceError('Unknown ledger file', 'not_found')
    return {'path': name, 'content': docs[name].decode(), 'revision': revision(docs)}


def mutate(path, request):
    root, _ = documents(path)
    lock_path = root / '.beancount.lock'
    fd = os.open(lock_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        root, docs = documents(path)
        before_revision = revision(docs)
        if not request.get('expected_revision') or request['expected_revision'] != before_revision:
            raise WorkspaceError('The ledger changed. Refresh and review your edit before saving again.', 'aborted')
        operation = request['operation']
        current = snapshot(path)
        name, content = '', ''
        if operation == 'save_transaction':
            source = request.get('source', '').strip() + '\n'
            if len(source.encode()) > 65536 or security_lines(source):
                raise WorkspaceError('Supply a single transaction without include or plugin directives')
            parsed, errors, _ = parser.parse_string(source)
            if errors or len(parsed) != 1 or not isinstance(parsed[0], data.Transaction):
                raise WorkspaceError('Supply exactly one valid Beancount transaction')
            if request.get('id'):
                tx = next((t for t in current['transactions'] if t['id'] == request['id']), None)
                if not tx:
                    raise WorkspaceError('Transaction no longer exists', 'not_found')
                name = tx['file']
                lines, start, end = span(docs[name].decode(), tx['line'])
                content = ''.join(lines[:start]) + source + ''.join(lines[end:])
            else:
                name = request.get('file') or Path(path).name
                if name not in docs:
                    raise WorkspaceError('Choose an existing ledger file', 'not_found')
                content = docs[name].decode().rstrip() + '\n\n' + source
        elif operation == 'delete_transaction':
            tx = next((t for t in current['transactions'] if t['id'] == request.get('id')), None)
            if not tx:
                raise WorkspaceError('Transaction no longer exists', 'not_found')
            name = tx['file']
            lines, start, end = span(docs[name].decode(), tx['line'])
            content = ''.join(lines[:start] + lines[end:])
        elif operation in ('open_account', 'close_account'):
            account = request.get('name', '')
            require_account(account)
            date = valid_date(request.get('date', ''))
            existing = next((a for a in current['accounts'] if a['name'] == account), None)
            name = 'accounts.beancount' if 'accounts.beancount' in docs else Path(path).name
            if operation == 'open_account':
                if existing:
                    raise WorkspaceError('Account already exists', 'already_exists')
                currencies = request.get('currencies', [])
                if not currencies or any(not re.fullmatch(r'[A-Z][A-Z0-9._-]*', c) for c in currencies):
                    raise WorkspaceError('Specify at least one valid currency')
                line = f'{date} open {account} ' + ','.join(dict.fromkeys(currencies))
            else:
                if not existing or existing['closed']:
                    raise WorkspaceError('Account is missing or already closed')
                if any(b['account'] == account and any(Decimal(p['units']['number']) != 0 for p in b['positions']) for b in current['balances']):
                    raise WorkspaceError('Account must have a zero balance before closing')
                line = f'{date} close {account}'
            content = docs[name].decode().rstrip() + '\n\n' + line + '\n'
        elif operation == 'write_file':
            name, content = request.get('path', ''), request.get('content', '')
        elif operation == 'restore_version':
            identifier = request.get('id', '')
            if not re.fullmatch(r'[0-9TZ.-]+-[a-f0-9]{12}', identifier):
                raise WorkspaceError('Invalid history identifier')
            file = root / '.beancount-history' / (identifier + '.json')
            if not file.is_file() or file.is_symlink():
                raise WorkspaceError('History entry not found', 'not_found')
            item = json.loads(file.read_text())
            name, content = item['file'], item['before_content']
        else:
            raise WorkspaceError('Unsupported mutation')
        if name not in docs:
            raise WorkspaceError('Choose an existing ledger file', 'not_found')
        if len(content.encode()) > 2 * 1024 * 1024:
            raise WorkspaceError('File exceeds the 2 MiB editing limit')
        if security_lines(content) != security_lines(docs[name].decode()):
            raise WorkspaceError('Include and plugin configuration can only be changed by the server operator')
        if content.encode() == docs[name]:
            return {'revision': before_revision, 'message': 'No changes'}
        with tempfile.TemporaryDirectory(prefix='beancount-validate-') as directory:
            stage = Path(directory)
            for rel, raw in docs.items():
                out = stage / rel
                out.parent.mkdir(parents=True, exist_ok=True)
                out.write_bytes(content.encode() if rel == name else raw)
            _, errors, _ = load(stage / Path(path).name)
            if errors:
                raise WorkspaceError('Validation failed: ' + '; '.join(e.message for e in errors[:10]), 'failed_precondition')
        _, now = documents(path)
        if revision(now) != before_revision:
            raise WorkspaceError('External file edit detected. Refresh before saving.', 'aborted')
        target = root / name
        after = dict(docs)
        after[name] = content.encode()
        after_revision = revision(after)
        timestamp = dt.datetime.now(dt.timezone.utc).isoformat().replace('+00:00', 'Z')
        identifier = dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S.%fZ') + '-' + uuid.uuid4().hex[:12]
        directory = root / '.beancount-history'
        if directory.is_symlink():
            raise WorkspaceError('Invalid history directory')
        directory.mkdir(mode=0o700, exist_ok=True)
        record = {'id': identifier, 'timestamp': timestamp, 'actor': request.get('actor', 'unknown'),
                  'operation': operation, 'file': name, 'before_revision': before_revision,
                  'after_revision': after_revision, 'before_content': docs[name].decode()}
        # Write the durable recovery record before replacing the ledger file.
        with (directory / (identifier + '.json')).open('x', encoding='utf-8') as f:
            os.chmod(f.name, 0o600)
            json.dump(record, f); f.flush(); os.fsync(f.fileno())
        directory_fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try: os.fsync(directory_fd)
        finally: os.close(directory_fd)
        fd, temporary = tempfile.mkstemp(prefix='.beancount-', dir=target.parent)
        try:
            with os.fdopen(fd, 'w', encoding='utf-8') as f:
                f.write(content); f.flush(); os.fsync(f.fileno())
            os.chmod(temporary, target.stat().st_mode & 0o777)
            os.replace(temporary, target)
            for d in (target.parent, directory):
                directory_fd = os.open(d, os.O_RDONLY | os.O_DIRECTORY)
                try: os.fsync(directory_fd)
                finally: os.close(directory_fd)
        finally:
            if os.path.exists(temporary): os.unlink(temporary)
        return {'revision': after_revision, 'message': 'Saved and validated'}


def report(path, request):
    snap = snapshot(path)
    if snap['errors']:
        raise WorkspaceError('Fix ledger validation errors before viewing reports', 'failed_precondition')
    start, end = request.get('from', ''), request.get('to', '')
    for value in (start, end):
        if value: valid_date(value)
    if start and end and start > end:
        raise WorkspaceError('Start date must precede end date')
    currency = request.get('currency') or (snap['operating_currencies'] or snap['currencies'] or ['USD'])[0]
    totals, period_totals, monthly = {}, {}, {}
    for tx in snap['transactions']:
        if end and tx['date'] > end: continue
        month = tx['date'][:7]
        for posting in tx['postings']:
            v = posting['book_value']
            if not v or v['currency'] != currency: continue
            n, name = Decimal(v['number']), posting['account']
            totals[name] = totals.get(name, Decimal(0)) + n
            m = monthly.setdefault(month, {'income': Decimal(0), 'expenses': Decimal(0), 'worth_change': Decimal(0)})
            if name.startswith(('Assets:', 'Liabilities:')): m['worth_change'] += n
            if not start or tx['date'] >= start:
                period_totals[name] = period_totals.get(name, Decimal(0)) + n
                if name.startswith('Income:'): m['income'] -= n
                if name.startswith('Expenses:'): m['expenses'] += n
    def total(prefix, values=totals):
        return sum((v for k, v in values.items() if k.startswith(prefix + ':')), Decimal(0))
    income, expenses = -total('Income', period_totals), total('Expenses', period_totals)
    assets, liabilities, equity = total('Assets'), -total('Liabilities'), -total('Equity')
    periods, worth = [], Decimal(0)
    # Fill missing months so charts never imply continuous activity across gaps.
    dates = [t['date'] for t in snap['transactions'] if not end or t['date'] <= end]
    first = (start or min(dates, default=end or dt.date.today().isoformat()))[:7]
    last = (end or max(dates, default=start or dt.date.today().isoformat()))[:7]
    worth = sum((v['worth_change'] for k, v in monthly.items() if k < first), Decimal(0))
    year, month = map(int, first.split('-'))
    for _ in range(1200):
        label = f'{year:04}-{month:02}'
        if label > last: break
        m = monthly.get(label, {'income': Decimal(0), 'expenses': Decimal(0), 'worth_change': Decimal(0)})
        worth += m['worth_change']
        periods.append({'month': label, 'income': str(m['income']), 'expenses': str(m['expenses']),
                        'net_income': str(m['income']-m['expenses']), 'net_worth': str(worth)})
        month += 1
        if month == 13: year, month = year+1, 1
    rows = lambda values, prefixes: [{'account': name, 'number': str(value)} for name, value in sorted(values.items()) if name.split(':')[0] in prefixes]
    return {'revision': snap['revision'], 'currency': currency, 'from': start, 'to': end,
            'income': str(income), 'expenses': str(expenses), 'net_income': str(income-expenses),
            'assets': str(assets), 'liabilities': str(liabilities), 'equity': str(equity),
            'net_worth': str(assets-liabilities),
            'income_statement': rows(period_totals, ['Income', 'Expenses']),
            'balance_sheet': rows(totals, ['Assets', 'Liabilities', 'Equity']),
            'trial_balance': rows(totals, ['Assets','Liabilities','Equity','Income','Expenses']), 'periods': periods}
