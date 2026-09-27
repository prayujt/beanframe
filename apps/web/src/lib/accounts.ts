import Decimal from 'decimal.js';
import type { Account, Report, ReportRow } from './gen/beancount/v1/ledger_pb';

export function accountComposition(
  rows: ReportRow[],
  group: 'Assets' | 'Liabilities' | 'Expenses'
) {
  const entries = rows.filter(
    (row) =>
      row.account.startsWith(group + ':') && !new Decimal(row.number).isZero()
  );
  // Skip a shared country or institution prefix to expose useful categories.
  let depth = 2;
  while (
    entries.length > 0 &&
    entries.every((row) => row.account.split(':').length > depth) &&
    new Set(
      entries.map((row) => row.account.split(':').slice(0, depth).join(':'))
    ).size === 1
  )
    depth++;
  const values = new Map<string, Decimal>();
  for (const row of entries) {
    const path = row.account.split(':').slice(0, depth).join(':');
    const amount = new Decimal(row.number).mul(
      group === 'Liabilities' ? -1 : 1
    );
    values.set(path, (values.get(path) || new Decimal(0)).plus(amount));
  }
  return [...values]
    .filter(([, amount]) => !amount.isZero())
    .map(([path, amount]) => ({
      path,
      label: path.split(':').at(-1) || path,
      amount
    }))
    .sort((a, b) => b.amount.abs().comparedTo(a.amount.abs()));
}

// Presentation names are deliberately separate from Beancount's account roots.
export const accountGroups = [
  'Assets',
  'Liabilities',
  'Equity',
  'Income',
  'Expenses'
];
export const accountLabel = (path: string) =>
  path === 'Income' ? 'Revenue' : path.split(':').at(-1) || path;
export function matchesAccount(path: string, query: string) {
  const haystack =
    `${path} ${path.startsWith('Income') ? 'revenue' : ''}`.toLowerCase();
  return query
    .toLowerCase()
    .split(/[\s:]+/)
    .filter(Boolean)
    .every((part) => haystack.includes(part));
}
export function accountPaths(accounts: Account[]) {
  const paths = new Set<string>();
  for (const account of accounts) {
    const parts = account.name.split(':');
    parts.forEach((_, i) => paths.add(parts.slice(0, i + 1).join(':')));
  }
  return [...paths].sort(
    (a, b) =>
      accountGroups.indexOf(a.split(':')[0]) -
        accountGroups.indexOf(b.split(':')[0]) || a.localeCompare(b)
  );
}
export type AccountNode = {
  path: string;
  label: string;
  own: Decimal;
  total: Decimal;
  children: AccountNode[];
  depth: number;
  closed: boolean;
};
export function accountTree(
  accounts: Account[],
  report?: Report
): AccountNode[] {
  const amounts = new Map<string, string>();
  for (const row of report?.trialBalance || [])
    if (!/^(Income|Expenses):/.test(row.account))
      amounts.set(row.account, row.number);
  for (const row of report?.incomeStatement || [])
    amounts.set(row.account, row.number);
  const nodes = new Map<string, AccountNode>();
  for (const path of accountPaths(accounts)) {
    const value = new Decimal(amounts.get(path) || 0);
    const normalCredit = /^(Liabilities|Equity|Income)(:|$)/.test(path);
    const own = normalCredit ? value.negated() : value;
    const account = accounts.find((a) => a.name === path);
    nodes.set(path, {
      path,
      label: accountLabel(path),
      own,
      total: own,
      children: [],
      depth: path.split(':').length - 1,
      closed: !!account?.closed
    });
  }
  const roots: AccountNode[] = [];
  for (const node of nodes.values()) {
    const parent = nodes.get(node.path.split(':').slice(0, -1).join(':'));
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  function rollup(node: AccountNode): Decimal {
    node.total = node.children.reduce(
      (sum, child) => sum.plus(rollup(child)),
      node.own
    );
    return node.total;
  }
  roots.forEach(rollup);
  return roots;
}
