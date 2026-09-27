<script lang="ts">
  import { tick } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    Search,
    CornerDownLeft,
    Wallet,
    FileText,
    ArrowUpRight
  } from '@lucide/svelte';
  import type {
    Account,
    Posting,
    Transaction
  } from '$lib/gen/beancount/v1/ledger_pb';
  import { accountPaths, matchesAccount } from '$lib/accounts';
  import { dateLabel } from '$lib/format';
  import Modal from './Modal.svelte';
  let {
    open = $bindable(false),
    accounts,
    transactions,
    nav,
    initialScope = 'all',
    onTransaction
  } = $props<{
    open: boolean;
    accounts: Account[];
    transactions: Transaction[];
    nav: { path: string; label: string }[];
    initialScope?: 'all' | 'accounts' | 'transactions';
    onTransaction: (transaction: Transaction) => void;
  }>();
  let query = $state('');
  let scope = $state('all');
  let active = $state(0);
  let input: HTMLInputElement;
  const id = $props.id();
  type Result = {
    id: string;
    kind: string;
    label: string;
    detail: string;
    path?: string;
    transaction?: Transaction;
  };
  const results = $derived.by(() => {
    const rows: Result[] = [];
    if (scope !== 'transactions') {
      if (scope === 'all')
        for (const item of nav.filter(
          (n: { path: string; label: string }) =>
            !query || n.label.toLowerCase().includes(query.toLowerCase())
        ))
          rows.push({
            id: 'page-' + item.path,
            kind: 'Pages',
            label: item.label,
            detail: '',
            path: item.path === 'overview' ? '/' : '/' + item.path
          });
      for (const path of accountPaths(accounts)
        .filter((a) => matchesAccount(a, query))
        .slice(0, query || scope === 'accounts' ? 50 : 8))
        rows.push({
          id: path,
          kind: 'Accounts',
          label: path.split(':').at(-1) || path,
          detail: path,
          path: '/journal?account=' + encodeURIComponent(path)
        });
    }
    if (scope !== 'accounts' && (query || scope === 'transactions')) {
      const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
      for (const tx of transactions
        .toReversed()
        .filter((t: Transaction) =>
          terms.every((word) =>
            `${t.payee} ${t.narration} ${t.postings.map((p: Posting) => p.account).join(' ')} ${t.tags.join(' ')}`
              .toLowerCase()
              .includes(word)
          )
        )
        .slice(0, 20))
        rows.push({
          id: tx.id,
          kind: 'Transactions',
          label: tx.payee || tx.narration,
          detail: `${dateLabel(tx.date)} · ${tx.narration}`,
          transaction: tx
        });
    }
    return rows;
  });
  $effect(() => {
    if (open) {
      query = '';
      scope = initialScope;
      active = 0;
    }
  });
  $effect(() => {
    query;
    scope;
    active = 0;
  });
  function choose(result: Result) {
    open = false;
    if (result.transaction) onTransaction(result.transaction);
    else if (result.path) void goto(result.path);
  }
  async function keydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      active =
        (active + (event.key === 'ArrowDown' ? 1 : -1) + results.length) %
        Math.max(1, results.length);
      await tick();
      document
        .getElementById(`${id}-${active}`)
        ?.scrollIntoView({ block: 'nearest' });
    } else if (event.key === 'Enter' && results[active]) {
      event.preventDefault();
      choose(results[active]);
    }
  }
</script>

<Modal
  bind:open
  title="Search workspace"
  description="Find accounts, transactions, and pages."
  kind="command"
  onOpenAutoFocus={(event) => {
    event.preventDefault();
    input?.focus();
  }}
>
  <div class="command-input">
    <Search size={18} /><input
      bind:this={input}
      bind:value={query}
      aria-label="Find a page or account"
      role="combobox"
      aria-expanded="true"
      aria-controls={`${id}-results`}
      aria-autocomplete="list"
      aria-activedescendant={results[active] ? `${id}-${active}` : undefined}
      placeholder="Search accounts, transactions, pages…"
      onkeydown={keydown}
      autocomplete="off"
    /><kbd>esc</kbd>
  </div>
  <div class="command-tabs" aria-label="Search category">
    {#each [['all', 'All'], ['accounts', 'Accounts'], ['transactions', 'Transactions']] as [value, label]}<button
        type="button"
        class:active={scope === value}
        aria-pressed={scope === value}
        onclick={() => {
          scope = value;
          input.focus();
        }}>{label}</button
      >{/each}<span>{results.length} results</span>
  </div>
  <div
    class="command-results"
    role="listbox"
    id={`${id}-results`}
    aria-label="Search results"
  >
    {#each results as result, i (result.id)}
      {#if i === 0 || results[i - 1].kind !== result.kind}<div
          class="command-group"
          aria-hidden="true"
        >
          {result.kind}
        </div>{/if}
      <button
        type="button"
        role="option"
        id={`${id}-${i}`}
        aria-selected={active === i}
        tabindex="-1"
        class:active={active === i}
        onpointermove={() => (active = i)}
        onpointerdown={(e) => e.preventDefault()}
        onclick={() => choose(result)}
      >
        <span class="command-result-icon"
          >{#if result.kind === 'Accounts'}<Wallet
              size={15}
            />{:else if result.kind === 'Transactions'}<ArrowUpRight
              size={15}
            />{:else}<FileText size={15} />{/if}</span
        ><span class="command-result-label"
          >{result.label}{#if result.detail}<small>{result.detail}</small
            >{/if}</span
        >{#if active === i}<CornerDownLeft size={13} />{/if}
      </button>
    {:else}<div class="command-empty">
        <Search size={22} /><strong>No results for “{query}”</strong><span
          >Try a payee, account name, or part of a description.</span
        >
      </div>{/each}
  </div>
  <div class="command-footer">
    <span><kbd>↑</kbd><kbd>↓</kbd> navigate</span><span><kbd>↵</kbd> open</span
    ><span>Search includes account groups and subaccounts</span>
  </div>
</Modal>
