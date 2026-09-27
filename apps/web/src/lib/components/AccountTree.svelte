<script lang="ts">
  import {
    ChevronRight,
    Search,
    ChevronsDownUp,
    ChevronsUpDown,
    ArrowUpRight
  } from '@lucide/svelte';
  import { accountTree, matchesAccount, type AccountNode } from '$lib/accounts';
  import { money } from '$lib/format';
  import type { Account, Report } from '$lib/gen/beancount/v1/ledger_pb';
  let { accounts, report, currency } = $props<{
    accounts: Account[];
    report: Report | undefined;
    currency: string;
  }>();
  let expanded = $state<string[]>([
    'Assets',
    'Liabilities',
    'Equity',
    'Income',
    'Expenses'
  ]);
  let query = $state('');
  let showZero = $state(false);
  const roots = $derived(accountTree(accounts, report));
  const rows = $derived.by(() => {
    const out: AccountNode[] = [];
    const visibility = new Map<string, boolean>();
    function relevant(node: AccountNode): boolean {
      const cached = visibility.get(node.path);
      if (cached !== undefined) return cached;
      const childrenMatch = node.children.some(relevant);
      const visible =
        (showZero ||
          !node.own.isZero() ||
          childrenMatch ||
          !node.total.isZero()) &&
        (!query || matchesAccount(node.path, query) || childrenMatch);
      visibility.set(node.path, visible);
      return visible;
    }
    function walk(nodes: AccountNode[]) {
      for (const node of nodes) {
        if (!relevant(node)) continue;
        out.push(node);
        if (query || expanded.includes(node.path)) walk(node.children);
      }
    }
    walk(roots);
    return out;
  });
  function toggle(path: string) {
    expanded = expanded.includes(path)
      ? expanded.filter((p) => p !== path)
      : [...expanded, path];
  }
  function expandAll() {
    const paths: string[] = [];
    function walk(nodes: AccountNode[]) {
      for (const n of nodes) {
        paths.push(n.path);
        walk(n.children);
      }
    }
    walk(roots);
    expanded = paths;
  }
</script>

<section class="panel account-breakdown" aria-label="Account breakdown">
  <div class="panel-heading">
    <div>
      <h2>Account breakdown</h2>
      <p class="muted small">Balances by account and subaccount.</p>
    </div>
    <a href="/accounts" class="text-link"
      >All accounts<ArrowUpRight size={13} /></a
    >
  </div>
  <div class="tree-toolbar">
    <label class="search-field"
      ><Search size={13} /><input
        bind:value={query}
        aria-label="Search account breakdown"
        placeholder="Filter accounts…"
      /></label
    ><label class="checkbox-label"
      ><input type="checkbox" bind:checked={showZero} />Show zero balances</label
    >
    <div class="tree-controls">
      <button
        class="icon-button"
        aria-label="Expand all accounts"
        title="Expand all"
        onclick={expandAll}><ChevronsUpDown size={14} /></button
      ><button
        class="icon-button"
        aria-label="Collapse all accounts"
        title="Collapse all"
        onclick={() => (expanded = [])}><ChevronsDownUp size={14} /></button
      >
    </div>
  </div>
  {#if !report}<p class="empty-inline">
      Account totals are available when the ledger is valid.
    </p>{:else}
    <div class="table-scroll">
      <table class="account-tree">
        <thead
          ><tr
            ><th>Account</th><th class="numeric"
              >Total <span class="muted">{currency}</span></th
            ></tr
          ></thead
        ><tbody>
          {#each rows as node (node.path)}<tr
              class:account-root={node.depth === 0}
              ><td
                ><div class="tree-account" style={`--depth:${node.depth}`}>
                  {#if node.children.length}<button
                      class="icon-button tree-toggle"
                      aria-label={`${expanded.includes(node.path) || query ? 'Collapse' : 'Expand'} ${node.path}`}
                      aria-expanded={!!query || expanded.includes(node.path)}
                      disabled={!!query}
                      onclick={() => toggle(node.path)}
                      ><ChevronRight
                        size={13}
                        class={expanded.includes(node.path) || query
                          ? 'rotated'
                          : ''}
                      /></button
                    >{:else}<span class="tree-indent"></span>{/if}
                  <a
                    href={`/journal?account=${encodeURIComponent(node.path)}`}
                    title={`${node.path} · Direct postings: ${money(node.own.toString(), currency)}`}
                    >{node.label}</a
                  >{#if node.closed}<span class="tree-closed">Closed</span>{/if}
                </div></td
              ><td class="numeric"
                ><a href={`/journal?account=${encodeURIComponent(node.path)}`}
                  >{money(node.total.toString(), currency)}</a
                ></td
              ></tr
            >{:else}<tr
              ><td colspan="2" class="empty-inline"
                >No accounts match these filters.</td
              ></tr
            >{/each}
        </tbody>
      </table>
    </div>{/if}
  <div class="tree-footnote">
    Revenue and expenses show the selected period. Assets, liabilities, and
    equity show closing balances at book cost. Parent totals include
    subaccounts.
  </div>
</section>
