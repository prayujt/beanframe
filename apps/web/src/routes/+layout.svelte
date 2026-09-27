<script>
  import '$lib/styles.css';
  let { children } = $props();
</script>

<svelte:head><title>Ledger</title></svelte:head>

{@render children()}
