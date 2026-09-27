<script lang="ts">
  import { DateRangePicker, Popover } from 'bits-ui';
  import {
    parseDate,
    today,
    getLocalTimeZone,
    startOfMonth,
    endOfMonth,
    startOfYear,
    type DateValue
  } from '@internationalized/date';
  import {
    CalendarDays,
    ChevronLeft,
    ChevronRight,
    ChevronDown
  } from '@lucide/svelte';
  let { from = $bindable(''), to = $bindable('') } = $props<{
    from: string;
    to: string;
  }>();
  let open = $state(false);
  let draft = $state<{
    start: DateValue | undefined;
    end: DateValue | undefined;
  }>({ start: undefined, end: undefined });
  let placeholder = $state<DateValue>(today(getLocalTimeZone()));
  let months = $state(2);
  function opened(value: boolean) {
    if (!value) return;
    draft = {
      start: from ? parseDate(from) : undefined,
      end: to ? parseDate(to) : undefined
    };
    placeholder = draft.start || today(getLocalTimeZone());
    months = window.innerWidth < 700 ? 1 : 2;
  }
  function label(value: string) {
    return parseDate(value)
      .toDate(getLocalTimeZone())
      .toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric'
      });
  }
  function preset(name: string) {
    const now = today(getLocalTimeZone());
    let start: DateValue | undefined;
    let end: DateValue | undefined = now;
    if (name === 'This month') start = startOfMonth(now);
    else if (name === 'Last month') {
      start = startOfMonth(now.subtract({ months: 1 }));
      end = endOfMonth(start);
    } else if (name === 'This quarter')
      start = startOfMonth(
        now.set({ month: Math.floor((now.month - 1) / 3) * 3 + 1 })
      );
    else if (name === 'Year to date') start = startOfYear(now);
    else if (name === 'Last 30 days') start = now.subtract({ days: 29 });
    else end = undefined;
    from = start?.toString() || '';
    to = end?.toString() || '';
    open = false;
  }
</script>

<DateRangePicker.Root
  bind:open
  onOpenChange={opened}
  bind:value={draft}
  bind:placeholder
  numberOfMonths={months}
  fixedWeeks
  weekdayFormat="short"
  closeOnRangeSelect={false}
  calendarLabel="Report date range"
>
  <DateRangePicker.Trigger
    class="button date-range-trigger"
    aria-label="Report date range"
    ><CalendarDays size={14} /><span
      >{from && to ? `${label(from)} – ${label(to)}` : 'All time'}</span
    ><ChevronDown size={12} /></DateRangePicker.Trigger
  >
  <Popover.Portal>
    <DateRangePicker.Content
      class="calendar-popover range-popover"
      sideOffset={8}
      align="start"
      collisionPadding={12}
    >
      <div class="date-presets" aria-label="Date presets">
        {#each ['All time', 'This month', 'Last month', 'This quarter', 'Year to date', 'Last 30 days'] as name}<button
            type="button"
            onclick={() => preset(name)}>{name}</button
          >{/each}
      </div>
      <div class="range-calendars">
        <DateRangePicker.Calendar>
          {#snippet children({ months, weekdays })}
            <DateRangePicker.Header class="calendar-header"
              ><DateRangePicker.PrevButton
                class="icon-button"
                aria-label="Previous month"
                ><ChevronLeft size={15} /></DateRangePicker.PrevButton
              >
              <div class="calendar-selects">
                <DateRangePicker.MonthSelect
                  aria-label="Month"
                /><DateRangePicker.YearSelect aria-label="Year" />
              </div>
              <DateRangePicker.NextButton
                class="icon-button"
                aria-label="Next month"
                ><ChevronRight size={15} /></DateRangePicker.NextButton
              ></DateRangePicker.Header
            >
            <div class="calendar-months">
              {#each months as month}<DateRangePicker.Grid class="calendar-grid"
                  ><DateRangePicker.GridHead
                    ><DateRangePicker.GridRow
                      >{#each weekdays as day}<DateRangePicker.HeadCell
                          >{day}</DateRangePicker.HeadCell
                        >{/each}</DateRangePicker.GridRow
                    ></DateRangePicker.GridHead
                  ><DateRangePicker.GridBody
                    >{#each month.weeks as week}<DateRangePicker.GridRow
                        >{#each week as date}<DateRangePicker.Cell
                            {date}
                            month={month.value}
                            ><DateRangePicker.Day /></DateRangePicker.Cell
                          >{/each}</DateRangePicker.GridRow
                      >{/each}</DateRangePicker.GridBody
                  ></DateRangePicker.Grid
                >{/each}
            </div>
          {/snippet}
        </DateRangePicker.Calendar>
        <div class="calendar-footer">
          <span
            >{draft.start
              ? label(draft.start.toString())
              : 'Choose a start date'}{draft.end
              ? ` – ${label(draft.end.toString())}`
              : draft.start
                ? ' – Choose an end date'
                : ''}</span
          ><button
            type="button"
            class="button primary"
            disabled={!draft.start || !draft.end}
            onclick={() => {
              from = draft.start!.toString();
              to = draft.end!.toString();
              open = false;
            }}>Apply</button
          >
        </div>
      </div>
    </DateRangePicker.Content>
  </Popover.Portal>
</DateRangePicker.Root>
