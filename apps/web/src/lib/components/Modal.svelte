<script lang="ts">
  import { Dialog } from 'bits-ui';
  import { X } from '@lucide/svelte';
  import type { Snippet } from 'svelte';
  let {
    open = $bindable(false),
    title,
    description = '',
    children,
    wide = false,
    onClose,
    kind = 'default',
    onOpenAutoFocus
  } = $props<{
    open: boolean;
    title: string;
    description?: string;
    children: Snippet;
    wide?: boolean;
    onClose?: () => void;
    kind?: 'default' | 'command' | 'editor';
    onOpenAutoFocus?: (event: Event) => void;
  }>();
</script>

<Dialog.Root
  bind:open
  onOpenChange={(value) => {
    if (!value) onClose?.();
  }}
>
  <Dialog.Portal>
    <Dialog.Overlay class="modal-overlay" />
    <Dialog.Content
      class={`modal ${wide ? 'modal-wide' : ''} modal-${kind}`}
      {onOpenAutoFocus}
    >
      <div class="modal-heading">
        <div>
          <Dialog.Title class="modal-title">{title}</Dialog.Title
          ><Dialog.Description class="muted small"
            >{description}</Dialog.Description
          >
        </div>
        {#if kind !== 'command'}<Dialog.Close
            class="icon-button"
            aria-label="Close dialog"><X size={16} /></Dialog.Close
          >{/if}
      </div>
      {@render children()}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
