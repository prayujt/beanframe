<script lang="ts">
  import { tick } from 'svelte';
  import { ChevronDown, Check, Search, X } from '@lucide/svelte';
  import { matchesAccount } from '$lib/accounts';
  let {
    value = $bindable(''),
    accounts,
    label,
    placeholder = 'Find an account…',
    allowEmpty = false,
    required = false
  } = $props<{
    value: string;
    accounts: string[];
    label: string;
    placeholder?: string;
    allowEmpty?: boolean;
    required?: boolean;
  }>();
  const id = $props.id();
  let open = $state(false);
  let searching = $state(false);
  let active = $state(0);
  let above = $state(false);
  let maxHeight = $state(260);
  let input: HTMLInputElement;
  function positionOptions() {
    const rect = input.getBoundingClientRect();
    const dialog = input.closest('[role="dialog"]')?.getBoundingClientRect();
    const below =
      Math.min(window.innerHeight, dialog?.bottom ?? Infinity) -
      rect.bottom -
      12;
    const before = rect.top - Math.max(0, dialog?.top ?? 0) - 12;
    above = below < 260 && before > below;
    maxHeight = Math.max(60, Math.min(260, above ? before : below));
  }
  const choices = $derived(
    accounts
      .filter((a: string) => !searching || matchesAccount(a, value))
      .slice(0, 60)
  );
  function choose(account: string) {
    value = account;
    open = false;
    searching = false;
    input.focus();
    open = false;
  }
  async function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && open) {
      event.preventDefault();
      event.stopPropagation();
      open = false;
      return;
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      open = true;
      positionOptions();
      active = Math.max(
        0,
        Math.min(
          choices.length - 1,
          active + (event.key === 'ArrowDown' ? 1 : -1)
        )
      );
      await tick();
      document
        .getElementById(`${id}-${active}`)
        ?.scrollIntoView({ block: 'nearest' });
    } else if (event.key === 'Enter' && open && choices[active]) {
      event.preventDefault();
      choose(choices[active]);
    }
  }
</script>

<div class="account-picker">
  <div class="account-picker-input">
    <input
      bind:this={input}
      bind:value
      aria-label={label}
      role="combobox"
      aria-expanded={open}
      aria-controls={`${id}-list`}
      aria-autocomplete="list"
      aria-activedescendant={open && choices[active]
        ? `${id}-${active}`
        : undefined}
      {placeholder}
      {required}
      autocomplete="off"
      onfocus={() => {
        open = true;
        positionOptions();
        searching = false;
        active = 0;
        input.select();
      }}
      oninput={() => {
        open = true;
        positionOptions();
        searching = true;
        active = 0;
      }}
      onblur={() => {
        open = false;
      }}
      onkeydown={keydown}
    />
    {#if allowEmpty && value}<button
        type="button"
        class="icon-button"
        aria-label="Clear account filter"
        onclick={() => {
          value = '';
          open = false;
        }}><X size={12} /></button
      >{:else}<ChevronDown size={12} />{/if}
  </div>
  {#if open}<div
      class="account-options"
      class:above
      style:max-height={`${maxHeight}px`}
      id={`${id}-list`}
      role="listbox"
      aria-label="Matching accounts"
    >
      <div class="account-options-caption">
        <Search size={12} />Type any part of an account
      </div>
      {#if allowEmpty}<button
          type="button"
          role="option"
          aria-selected={!value}
          tabindex="-1"
          onpointerdown={(e) => e.preventDefault()}
          onclick={() => choose('')}>All accounts</button
        >{/if}
      {#each choices as name, i}<button
          type="button"
          role="option"
          id={`${id}-${i}`}
          aria-selected={value === name}
          tabindex="-1"
          class:highlighted={i === active}
          onpointerdown={(e) => e.preventDefault()}
          onpointermove={() => (active = i)}
          onclick={() => choose(name)}
          ><span>{name.split(':').at(-1)}<small>{name}</small></span
          >{#if name === value}<Check size={12} />{/if}</button
        >{:else}<div class="account-options-empty">
          No matching accounts
        </div>{/each}
    </div>{/if}
</div>
