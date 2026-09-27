<script lang="ts">
  import { BookOpen } from '@lucide/svelte';

  let { url = '', small = false }: { url?: string; small?: boolean } = $props();
  let failedURL = $state<string>();
</script>

<span
  class="brand-mark"
  class:small-mark={small}
  class:brand-image={url && failedURL !== url}
  aria-hidden="true"
>
  {#if url && failedURL !== url}
    <img
      src={url}
      alt=""
      referrerpolicy="no-referrer"
      onerror={() => (failedURL = url)}
    />
  {:else}
    <BookOpen size={small ? 16 : 22} strokeWidth={1.6} />
  {/if}
</span>
