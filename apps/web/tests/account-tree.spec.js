import { test, expect } from '@playwright/test';
import { create } from '@bufbuild/protobuf';
import {
  AccountSchema,
  ReportSchema
} from '../src/lib/gen/beancount/v1/ledger_pb';
import { accountTree, accountComposition } from '../src/lib/accounts';

test('account totals preserve decimals, parent postings, period boundaries and credit signs', () => {
  const accounts = [
    'Assets:Cash',
    'Assets:Cash:Savings',
    'Liabilities:Card',
    'Income:Sales',
    'Expenses:Software'
  ].map((name) => create(AccountSchema, { name }));
  const report = create(ReportSchema, {
    trialBalance: [
      { account: 'Assets:Cash', number: '0.1' },
      { account: 'Assets:Cash:Savings', number: '0.2' },
      { account: 'Liabilities:Card', number: '-20' },
      { account: 'Income:Sales', number: '-999' },
      { account: 'Expenses:Software', number: '999' }
    ],
    incomeStatement: [
      { account: 'Income:Sales', number: '-30' },
      { account: 'Expenses:Software', number: '12.34' }
    ]
  });
  const roots = accountTree(accounts, report);
  const assets = roots.find((n) => n.path === 'Assets');
  expect(assets?.total.toString()).toBe('0.3');
  expect(assets?.children[0].own.toString()).toBe('0.1');
  expect(assets?.children[0].total.toString()).toBe('0.3');
  expect(roots.find((n) => n.path === 'Liabilities')?.total.toString()).toBe(
    '20'
  );
  expect(roots.find((n) => n.path === 'Income')?.total.toString()).toBe('30');
  expect(roots.find((n) => n.path === 'Expenses')?.total.toString()).toBe(
    '12.34'
  );
});

test('composition groups useful account branches and preserves signed balances', () => {
  const rows = create(ReportSchema, {
    balanceSheet: [
      { account: 'Assets:US:Bank:Checking', number: '0.1' },
      { account: 'Assets:US:Bank:Savings', number: '0.2' },
      { account: 'Assets:US:Broker:Cash', number: '20' },
      { account: 'Liabilities:Card', number: '-100' },
      { account: 'Liabilities:Credit', number: '5' },
      { account: 'Liabilities:Unused', number: '0' }
    ]
  }).balanceSheet;
  expect(
    accountComposition(rows, 'Assets').map((c) => [c.path, c.amount.toString()])
  ).toEqual([
    ['Assets:US:Broker', '20'],
    ['Assets:US:Bank', '0.3']
  ]);
  expect(
    accountComposition(rows, 'Liabilities').map((c) => [
      c.path,
      c.amount.toString()
    ])
  ).toEqual([
    ['Liabilities:Card', '100'],
    ['Liabilities:Credit', '-5']
  ]);
  expect(accountComposition(rows, 'Expenses')).toEqual([]);
});
