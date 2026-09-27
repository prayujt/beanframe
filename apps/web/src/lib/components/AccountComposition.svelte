<script lang="ts">
  import Decimal from 'decimal.js';
  import type { ReportRow } from '$lib/gen/beancount/v1/ledger_pb';
  import { money } from '$lib/format';
  import { accountComposition } from '$lib/accounts';
  let {
    rows,
    currency,
    group = 'Expenses'
  } = $props<{
    rows: ReportRow[];
    currency: string;
    group?: 'Assets' | 'Liabilities' | 'Expenses';
  }>();
  const description = $derived(
    group === 'Expenses'
      ? 'Expense'
      : group === 'Assets'
        ? 'Asset'
        : 'Liability'
  );
  const colors = [
    '#8581c5',
    '#77a99d',
    '#a4b7ce',
    '#c4a480',
    '#b796ae',
    '#b3b5bf'
  ];
  let active = $state(-1);
  const categories = $derived(accountComposition(rows, group));
  const total = $derived(
    categories.reduce((sum, c) => sum.plus(c.amount), new Decimal(0))
  );
  const magnitude = $derived(
    categories.reduce((sum, c) => sum.plus(c.amount.abs()), new Decimal(0))
  );
  const segments = $derived.by(() => {
    let offset = 0;
    return categories.map((c, i) => {
      const share = magnitude.isZero()
        ? 0
        : c.amount.abs().div(magnitude).toNumber();
      const segment = { ...c, share, offset, color: colors[i % colors.length] };
      offset += share;
      return segment;
    });
  });
</script>

{#if segments.length}<div class="expense-composition">
    <div class="expense-ring">
      <svg
        viewBox="0 0 200 200"
        role="img"
        aria-label={`${description} composition by account category`}
      >
        {#each segments as segment, i}<circle
            cx="100"
            cy="100"
            r="76"
            fill="none"
            stroke={segment.color}
            stroke-width={active === i ? 26 : 22}
            pathLength="100"
            stroke-dasharray={`${Math.max(0, segment.share * 100 - 0.6)} ${100 - Math.max(0, segment.share * 100 - 0.6)}`}
            stroke-dashoffset={-segment.offset * 100}
            transform="rotate(-90 100 100)"
            opacity={active < 0 || active === i ? 1 : 0.3}
            ><title
              >{segment.label}: {money(
                segment.amount.toString(),
                currency
              )}</title
            ></circle
          >{/each}
      </svg>
      <div class="expense-ring-label">
        <span
          >{segments[active]?.label ||
            (group === 'Expenses' ? 'Net expenses' : group)}</span
        ><strong
          >{money(
            (segments[active]?.amount || total).toString(),
            currency
          )}</strong
        ><small
          >{active >= 0
            ? `${(segments[active].share * 100).toFixed(1)}% of category magnitudes`
            : `${segments.length} ${segments.length === 1 ? 'account' : 'categories'}`}</small
        >
      </div>
    </div>
    <div class="expense-chart-legend" aria-label={`${description} categories`}>
      {#each segments as segment, i}<a
          href={`/journal?account=${encodeURIComponent(segment.path)}`}
          onpointerenter={() => (active = i)}
          onpointerleave={() => (active = -1)}
          onfocus={() => (active = i)}
          onblur={() => (active = -1)}
          ><i style={`background:${segment.color}`}></i><span
            >{segment.label}</span
          ><strong>{money(segment.amount.toString(), currency)}</strong></a
        >{/each}
    </div>
  </div>
  {#if categories.some((c) => c.amount.isNegative())}<p
      class="expense-chart-note"
    >
      Ring sizes show category magnitudes; negative balances keep their signs.
    </p>{/if}
{:else}<div class="chart-empty">
    {group === 'Expenses'
      ? 'No expenses in this period'
      : `No ${group.toLowerCase()} balances to display`}
  </div>{/if}
