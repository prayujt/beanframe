<script lang="ts">
  import { Plus, Trash2, Code2, List } from '@lucide/svelte';
  import type {
    Account,
    Transaction,
    FileInfo
  } from '$lib/gen/beancount/v1/ledger_pb';
  import Modal from './Modal.svelte';
  import AccountPicker from './AccountPicker.svelte';
  import SourcePreview from './SourcePreview.svelte';
  import DateControl from './DateControl.svelte';
  import Decimal from 'decimal.js';
  let {
    open = $bindable(false),
    transaction,
    accounts,
    files,
    revision,
    save,
    saving,
    error,
    conflict
  } = $props<{
    open: boolean;
    transaction: Transaction | null;
    accounts: Account[];
    files: FileInfo[];
    revision: string;
    save: (value: {
      id: string;
      source: string;
      file: string;
      expectedRevision: string;
    }) => Promise<void>;
    saving: boolean;
    error: string;
    conflict: boolean;
  }>();
  let mode = $state('form');
  let date = $state('');
  let flag = $state('*');
  let payee = $state('');
  let narration = $state('');
  let source = $state('');
  let file = $state('');
  let baseRevision = $state('');
  let postings = $state<
    { account: string; number: string; currency: string }[]
  >([]);
  let opened = false;
  $effect(() => {
    if (open && !opened) {
      mode = transaction ? 'source' : 'form';
      date = transaction?.date || new Date().toISOString().slice(0, 10);
      flag = transaction?.flag || '*';
      payee = transaction?.payee || '';
      narration = transaction?.narration || '';
      source = transaction?.source || '';
      file =
        transaction?.file ||
        files.find(
          (f: FileInfo) =>
            f.path === `transactions/${date.slice(0, 4)}.beancount`
        )?.path ||
        files.find((f: FileInfo) => f.path.includes('transactions'))?.path ||
        files.find((f: FileInfo) => f.path === 'main.beancount')?.path ||
        files[0]?.path ||
        '';
      baseRevision = revision;
      postings = [
        {
          account:
            accounts.find(
              (a: Account) => a.name === 'Expenses:Uncategorized' && !a.closed
            )?.name || '',
          number: '',
          currency: 'USD'
        },
        {
          account:
            accounts.find(
              (a: Account) =>
                a.name.startsWith('Assets:') &&
                a.name.endsWith(':Checking') &&
                !a.closed
            )?.name ||
            accounts.find(
              (a: Account) =>
                a.name.startsWith('Assets:') &&
                /:(Cash|Bank)$/.test(a.name) &&
                !a.closed
            )?.name ||
            '',
          number: '',
          currency: 'USD'
        }
      ];
    }
    opened = open;
  });
  let formatted = $derived(
    `${date} ${flag} ${payee ? JSON.stringify(payee) + ' ' : ''}${JSON.stringify(narration)}\n${postings.map((p) => `  ${p.account}  ${p.number ? p.number + ' ' + p.currency : ''}`).join('\n')}\n`
  );
  let balance = $derived.by(() => {
    try {
      if (postings.filter((p) => !p.number.trim()).length > 1)
        return 'Enter amounts; one posting may be inferred';
      const totals: Record<string, Decimal> = {};
      for (const p of postings) {
        if (!p.number) return 'One posting will be inferred';
        totals[p.currency] = (totals[p.currency] || new Decimal(0)).plus(
          p.number
        );
      }
      return Object.values(totals).every((v) => v.isZero())
        ? 'Balanced'
        : Object.entries(totals)
            .map(([c, n]) => `${n.toString()} ${c}`)
            .join(', ') + ' remaining';
    } catch {
      return 'Enter valid decimal amounts';
    }
  });
  function submit(event: SubmitEvent) {
    event.preventDefault();
    void save({
      id: transaction?.id || '',
      source: mode === 'source' ? source : formatted,
      file,
      expectedRevision: baseRevision
    });
  }
</script>

<Modal
  bind:open
  title={transaction ? 'Edit transaction' : 'New transaction'}
  description="Every save is checked against the complete ledger."
  wide
  kind="editor"
>
  <form onsubmit={submit}>
    {#if error}<div class="alert error" role="alert">{error}</div>{/if}
    {#if conflict}<div class="alert warning">
        The ledger changed while this editor was open. Your draft is preserved.
        Close and reopen after reviewing the latest data.
      </div>{/if}
    <div class="editor-toolbar">
      <div class="segmented">
        <button
          type="button"
          class:active={mode === 'form'}
          disabled={!!transaction ||
            (mode === 'source' && source !== formatted)}
          onclick={() => (mode = 'form')}><List size={14} />Fields</button
        ><button
          type="button"
          class:active={mode === 'source'}
          onclick={() => {
            if (mode === 'form') source = formatted;
            mode = 'source';
          }}><Code2 size={14} />Source</button
        >
      </div>
      <label class="inline-label"
        >File <select bind:value={file} disabled={!!transaction}
          >{#each files as f}<option value={f.path}>{f.path}</option
            >{/each}</select
        ></label
      >
    </div>
    <div class="transaction-editor-layout">
      <div class="transaction-fields">
        {#if mode === 'form'}
          <div class="form-grid">
            <div class="field-label">
              <span>Date</span><DateControl
                bind:value={date}
                label="Transaction date"
              />
            </div>
            <label
              >Status<select bind:value={flag}
                ><option value="*">Cleared</option><option value="!"
                  >Pending</option
                ></select
              ></label
            >
          </div>
          <label
            >Payee<input
              bind:value={payee}
              placeholder="Who was this payment to or from?"
              maxlength="300"
            /></label
          >
          <label
            >Description<input
              bind:value={narration}
              placeholder="What was it for?"
              required
              maxlength="1000"
            /></label
          >
          <div class="section-label">
            Postings <span class:positive={balance === 'Balanced'}
              >{balance}</span
            >
          </div>
          <div class="postings-form">
            {#each postings as p, i}<div class="posting-row">
                <AccountPicker
                  label={`Posting ${i + 1} account`}
                  accounts={accounts
                    .filter((a: Account) => !a.closed)
                    .map((a: Account) => a.name)}
                  bind:value={p.account}
                  required
                />
                <input
                  aria-label={`Posting ${i + 1} amount`}
                  bind:value={p.number}
                  placeholder="Auto"
                  inputmode="decimal"
                /><input
                  aria-label={`Posting ${i + 1} currency`}
                  bind:value={p.currency}
                  placeholder="USD"
                  required
                /><button
                  type="button"
                  class="icon-button"
                  aria-label={`Remove posting ${i + 1}`}
                  disabled={postings.length <= 2}
                  onclick={() =>
                    (postings = postings.filter((_, j) => j !== i))}
                  ><Trash2 size={14} /></button
                >
              </div>{/each}
          </div>
          <button
            type="button"
            class="button ghost small"
            onclick={() =>
              (postings = [
                ...postings,
                { account: '', number: '', currency: 'USD' }
              ])}><Plus size={14} />Add posting</button
          >
        {:else}
          <label class="sr-only" for="transaction-source"
            >Transaction source</label
          ><textarea
            id="transaction-source"
            class="code-editor transaction-source"
            bind:value={source}
            spellcheck="false"
            required></textarea>
          <p class="muted small">
            Beancount syntax supports tags, metadata, split postings, prices,
            and cost lots. Enter exactly one transaction.
          </p>
        {/if}
      </div>
      <SourcePreview source={mode === 'form' ? formatted : source} {file} />
    </div>
    <div class="modal-footer">
      <button type="button" class="button" onclick={() => (open = false)}
        >Cancel</button
      ><button
        type="submit"
        class="button primary"
        disabled={saving ||
          conflict ||
          (mode === 'form' &&
            postings.filter((p) => !p.number.trim()).length > 1)}
        >{saving
          ? 'Validating…'
          : transaction
            ? 'Save transaction'
            : 'Add transaction'}</button
      >
    </div>
  </form>
</Modal>
