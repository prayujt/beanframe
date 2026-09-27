<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Copy, Check } from '@lucide/svelte';
  let { source, file } = $props<{ source: string; file: string }>();
  let copied = $state(false);
  let copyError = $state(false);
  let timer: ReturnType<typeof setTimeout>;
  onDestroy(() => clearTimeout(timer));
  const tokens = (line: string) =>
    line.split(
      /("(?:\\.|[^"\\])*"|\d{4}-\d{2}-\d{2}|(?:Assets|Liabilities|Equity|Income|Expenses)(?::[A-Z][A-Za-z0-9-]*)+|[-+]?\d+(?:\.\d+)?|\b[A-Z][A-Z0-9._-]*\b)/g
    );
  function tokenClass(token: string) {
    if (token.startsWith('"')) return 'syntax-string';
    if (/^\d{4}-/.test(token)) return 'syntax-date';
    if (token.includes(':')) return 'syntax-account';
    if (/^[-+]?\d/.test(token)) return 'syntax-number';
    if (/^[A-Z][A-Z0-9._-]*$/.test(token)) return 'syntax-currency';
    return '';
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(source);
      copied = true;
      copyError = false;
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1500);
    } catch {
      copyError = true;
    }
  }
</script>

<aside class="transaction-preview">
  <div class="source-preview-heading">
    <span>Source preview</span><button
      type="button"
      class="icon-button"
      aria-label="Copy source"
      title={copied ? 'Copied' : 'Copy source'}
      onclick={copy}
      >{#if copied}<Check size={13} />{:else}<Copy size={13} />{/if}</button
    >
  </div>
  <div class="source-preview-file">{file}</div>
  <pre class="live-source" aria-label="Live Beancount source"><code
      >{#each source.trimEnd().split('\n') as line, i}<span class="source-line"
          ><span class="source-line-number" aria-hidden="true">{i + 1}</span
          ><span
            >{#each tokens(line) as token}<span class={tokenClass(token)}
                >{token}</span
              >{/each}</span
          ></span
        >{/each}</code
    ></pre>
  <p class="source-preview-note">
    This is the exact source that will be saved. The complete ledger is
    validated before any file changes.
  </p>
  {#if copyError}<p class="source-preview-note">
      Select the source to copy it; clipboard access is unavailable.
    </p>{/if}
</aside>
