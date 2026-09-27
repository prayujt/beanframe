<script lang="ts">
  import { DatePicker } from 'bits-ui';
  import {
    parseDate,
    today,
    getLocalTimeZone,
    type DateValue
  } from '@internationalized/date';
  import { CalendarDays, ChevronLeft, ChevronRight } from '@lucide/svelte';
  let { value = $bindable(''), label = 'Date' } = $props<{
    value: string;
    label?: string;
  }>();
  let open = $state(false);
  let selected = $derived(value ? parseDate(value) : undefined);
  let placeholder = $state<DateValue>(today(getLocalTimeZone()));
</script>

<DatePicker.Root
  value={selected}
  bind:open
  bind:placeholder
  onValueChange={(date) => {
    if (date) value = date.toString();
  }}
  onOpenChange={(isOpen) => {
    if (isOpen) placeholder = selected || today(getLocalTimeZone());
  }}
  fixedWeeks
  weekdayFormat="short"
  calendarLabel={label}
>
  <DatePicker.Trigger class="button date-input-trigger" aria-label={label}
    ><CalendarDays size={14} /><span
      >{selected
        ? selected
            .toDate(getLocalTimeZone())
            .toLocaleDateString('en-US', {
              month: 'short',
              day: 'numeric',
              year: 'numeric'
            })
        : 'Choose date'}</span
    ></DatePicker.Trigger
  >
  <DatePicker.Portal
    ><DatePicker.Content
      class="calendar-popover single-calendar"
      sideOffset={6}
      align="start"
      collisionPadding={12}
    >
      <DatePicker.Calendar
        >{#snippet children({ months, weekdays })}
          <DatePicker.Header class="calendar-header"
            ><DatePicker.PrevButton
              class="icon-button"
              aria-label="Previous month"
              ><ChevronLeft size={15} /></DatePicker.PrevButton
            >
            <div class="calendar-selects">
              <DatePicker.MonthSelect
                aria-label="Month"
              /><DatePicker.YearSelect aria-label="Year" />
            </div>
            <DatePicker.NextButton class="icon-button" aria-label="Next month"
              ><ChevronRight size={15} /></DatePicker.NextButton
            ></DatePicker.Header
          >
          {#each months as month}<DatePicker.Grid class="calendar-grid"
              ><DatePicker.GridHead
                ><DatePicker.GridRow
                  >{#each weekdays as day}<DatePicker.HeadCell
                      >{day}</DatePicker.HeadCell
                    >{/each}</DatePicker.GridRow
                ></DatePicker.GridHead
              ><DatePicker.GridBody
                >{#each month.weeks as week}<DatePicker.GridRow
                    >{#each week as date}<DatePicker.Cell
                        {date}
                        month={month.value}><DatePicker.Day /></DatePicker.Cell
                      >{/each}</DatePicker.GridRow
                  >{/each}</DatePicker.GridBody
              ></DatePicker.Grid
            >{/each}
        {/snippet}</DatePicker.Calendar
      >
      <button
        type="button"
        class="button ghost calendar-today"
        onclick={() => {
          value = today(getLocalTimeZone()).toString();
          open = false;
        }}>Today</button
      >
    </DatePicker.Content></DatePicker.Portal
  >
</DatePicker.Root>
