<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto, beforeNavigate } from '$app/navigation';
  import {
    LayoutDashboard,
    BookOpen,
    Wallet,
    ChartNoAxesCombined,
    Scale,
    Files,
    CircleCheck,
    History as HistoryIcon,
    Plus,
    Search,
    ChevronRight,
    ArrowUpRight,
    ArrowRight,
    ArrowDownLeft,
    RefreshCw,
    Download,
    PanelLeftClose,
    PanelLeftOpen,
    Moon,
    Sun,
    LogOut,
    Settings2,
    CircleAlert,
    X,
    Circle,
    Command,
    ExternalLink,
    Copy,
    Pencil,
    Trash2,
    Undo2,
    Check,
    LoaderCircle
  } from '@lucide/svelte';
  import Decimal from 'decimal.js';
  import { api, errorMessage, isSignedOut } from './api';
  import { money, dateLabel, shortAccount, download, csv } from './format';
  import type {
    Snapshot,
    Session,
    Report,
    Transaction,
    LedgerFile,
    HistoryEntry,
    ReportRow
  } from './gen/beancount/v1/ledger_pb';
  import Chart from './components/Chart.svelte';
  import BrandMark from './components/BrandMark.svelte';
  import Modal from './components/Modal.svelte';
  import TransactionEditor from './components/TransactionEditor.svelte';
  import AccountTree from './components/AccountTree.svelte';
  import AccountComposition from './components/AccountComposition.svelte';
  import DateRangeControl from './components/DateRangeControl.svelte';
  import DateControl from './components/DateControl.svelte';
  import AccountPicker from './components/AccountPicker.svelte';
  import CommandPalette from './components/CommandPalette.svelte';
  import { accountPaths } from './accounts';

  const nav = [
    { path: 'overview', label: 'Overview', icon: LayoutDashboard },
    { path: 'journal', label: 'Journal', icon: BookOpen },
    { path: 'accounts', label: 'Accounts', icon: Wallet },
    { path: 'income', label: 'Income statement', icon: ChartNoAxesCombined },
    { path: 'balance', label: 'Balance sheet', icon: Scale },
    { path: 'files', label: 'Ledger files', icon: Files },
    { path: 'checks', label: 'Validation', icon: CircleCheck },
    { path: 'history', label: 'History', icon: HistoryIcon }
  ];
  const navGroups = [
    { label: 'Ledger', icon: BookOpen, items: nav.slice(0, 3), tone: 'ledger' },
    {
      label: 'Reports',
      icon: ChartNoAxesCombined,
      items: nav.slice(3, 5),
      tone: 'reports'
    },
    { label: 'Manage', icon: Files, items: nav.slice(5), tone: 'manage' }
  ];
  let collapsedGroups = $state<string[]>([]);
  const view = $derived(page.params.view || 'overview');
  const current = $derived(nav.find((n) => n.path === view));
  let session = $state<Session>();
  const brandName = $derived(session?.brandName || 'Beanframe');
  let snapshot = $state<Snapshot>();
  let report = $state<Report>();
  let loading = $state(true);
  let refreshing = $state(false);
  let error = $state('');
  let notice = $state('');
  let loginError = $state('');
  const currency = 'USD';
  let from = $state('');
  let to = $state('');
  let connection = $state('Connecting');
  let dark = $state(false);
  let sidebar = $state(true);
  let search = $state('');
  let accountFilter = $state('');
  let flagFilter = $state('');
  let pageIndex = $state(0);
  let accountSearch = $state('');
  let accountKind = $state('All');
  let transactionOpen = $state(false);
  let selectedTransaction = $state<Transaction | null>(null);
  let saving = $state(false);
  let mutationError = $state('');
  let editorRevision = $state('');
  let detail = $state<Transaction | null>(null);
  let detailOpen = $state(false);
  let accountOpen = $state(false);
  let accountName = $state('');
  let accountDate = $state(new Date().toISOString().slice(0, 10));
  let accountCurrencies = $state('USD');
  let accountClosing = $state(false);
  let file = $state<LedgerFile>();
  let draft = $state('');
  let fileLoading = $state(false);
  let fileError = $state('');
  let history = $state<HistoryEntry[]>([]);
  let confirmOpen = $state(false);
  let confirmTitle = $state('');
  let confirmDescription = $state('');
  let confirmAction = $state<() => Promise<void>>(async () => {});
  let settingsOpen = $state(false);
  let paletteOpen = $state(false);
  let paletteScope = $state<'all' | 'accounts' | 'transactions'>('all');
  let initialized = $state(false);
  let disposed = false;
  let refreshId = 0;
  let fileId = 0;
  let reportId = 0;
  let watchController: AbortController;
  let toastTimer: ReturnType<typeof setTimeout>;
  const dirty = $derived(!!file && draft !== file.content);
  const canAccess = $derived(
    !!session && (!session.authEnabled || session.authenticated)
  );
  const canWrite = $derived(!!session?.canWrite);
  const staleEditor = $derived(
    transactionOpen && !!editorRevision && editorRevision !== snapshot?.revision
  );
  const filteredTransactions = $derived(
    (snapshot?.transactions || [])
      .filter(
        (t) =>
          (!from || t.date >= from) &&
          (!to || t.date <= to) &&
          (!flagFilter || t.flag === flagFilter) &&
          (!accountFilter ||
            t.postings.some(
              (p) =>
                p.account === accountFilter ||
                p.account.startsWith(accountFilter + ':')
            )) &&
          (!search ||
            `${t.payee} ${t.narration} ${t.tags.join(' ')} ${t.postings.map((p) => p.account).join(' ')}`
              .toLowerCase()
              .includes(search.toLowerCase()))
      )
      .toReversed()
  );
  const transactions = $derived(
    filteredTransactions.slice(pageIndex * 50, (pageIndex + 1) * 50)
  );
  const visibleAccounts = $derived(
    (snapshot?.accounts || []).filter(
      (a) =>
        (accountKind === 'All' || a.name.startsWith(accountKind + ':')) &&
        a.name.toLowerCase().includes(accountSearch.toLowerCase())
    )
  );
  const monthLabels = $derived(
    (report?.periods || []).map((p) =>
      new Date(p.month + '-01T12:00:00Z').toLocaleDateString('en-US', {
        month: 'short',
        year: '2-digit',
        timeZone: 'UTC'
      })
    )
  );
  const flowData = $derived([
    {
      label: 'Revenue',
      data: (report?.periods || []).map((p) => Number(p.income)),
      backgroundColor: '#8bb9a9',
      borderRadius: 3,
      maxBarThickness: 18
    },
    {
      label: 'Expenses',
      data: (report?.periods || []).map((p) => Number(p.expenses)),
      backgroundColor: '#7774ce',
      borderRadius: 3,
      maxBarThickness: 18
    }
  ]);
  const worthData = $derived([
    {
      label: 'Net assets',
      data: (report?.periods || []).map((p) => Number(p.netWorth)),
      borderColor: '#7774ce',
      backgroundColor: 'rgba(119,116,206,.06)',
      fill: true,
      pointRadius: 2,
      pointHoverRadius: 5,
      tension: 0.25,
      borderWidth: 2
    }
  ]);

  const profitData = $derived([
    {
      label: 'Net profit',
      data: (report?.periods || []).map((p) => Number(p.netIncome)),
      backgroundColor: (report?.periods || []).map((p) =>
        new Decimal(p.netIncome || 0).isNegative() ? '#cb8584' : '#719e94'
      ),
      borderRadius: 3,
      maxBarThickness: 24
    }
  ]);

  function toast(value: string) {
    notice = value;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (notice = ''), 4500);
  }
  function signedOut() {
    if (session) session = { ...session, authenticated: false };
    connection = 'Signed out';
    watchController?.abort();
    error = 'Your session ended. Sign in again to continue.';
  }
  async function loadReport() {
    const id = ++reportId;
    if (snapshot?.errors.length) {
      report = undefined;
      return;
    }
    try {
      const result = await api.getReport({ from, to, currency });
      if (id === reportId) {
        report = result;
        error = '';
      }
    } catch (e) {
      if (id === reportId) {
        report = undefined;
        error = errorMessage(e);
        if (isSignedOut(e)) signedOut();
      }
    }
  }
  async function refresh() {
    const id = ++refreshId;
    refreshing = true;
    try {
      const result = await api.getSnapshot({});
      if (id !== refreshId || disposed) return;
      snapshot = result;
      await loadReport();
      if (view === 'history') await loadHistory();
      if (file && !dirty) await loadFile(file.path);
    } catch (e) {
      error = errorMessage(e);
      if (isSignedOut(e)) signedOut();
    } finally {
      if (id === refreshId) refreshing = false;
      loading = false;
    }
  }
  async function watch() {
    while (!disposed && !watchController.signal.aborted) {
      try {
        connection = 'Connecting';
        for await (const event of api.watchLedger(
          {},
          { signal: watchController.signal }
        )) {
          connection = 'Live';
          if (event.revision && event.revision !== snapshot?.revision)
            await refresh();
        }
      } catch (e) {
        if (watchController.signal.aborted) return;
        if (isSignedOut(e)) {
          signedOut();
          return;
        }
        connection = 'Reconnecting';
      }
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }
  }
  async function start() {
    loading = true;
    error = '';
    try {
      session = await api.getSession({});
      if (!session.authEnabled || session.authenticated) {
        await refresh();
        initialized = true;
        watchController = new AbortController();
        void watch();
      } else loading = false;
    } catch (e) {
      error = errorMessage(e);
      loading = false;
    }
  }
  let checkingSession = false;
  async function renewSession() {
    if (checkingSession || !canAccess || !session?.authEnabled) return;
    checkingSession = true;
    try {
      const latest = await api.getSession({});
      if (disposed) return;
      session = latest;
      if (!latest.authenticated) signedOut();
    } catch (e) {
      if (isSignedOut(e)) signedOut();
    } finally {
      checkingSession = false;
    }
  }
  async function loadHistory() {
    try {
      history = (await api.getHistory({})).entries;
    } catch (e) {
      error = errorMessage(e);
    }
  }
  async function loadFile(path: string) {
    if (dirty && !window.confirm('Discard the unsaved file changes?')) return;
    const id = ++fileId;
    const originalDraft = draft;
    fileLoading = true;
    fileError = '';
    try {
      const value = await api.readFile({ path });
      if (id === fileId && draft === originalDraft) {
        file = value;
        draft = value.content;
      }
    } catch (e) {
      fileError = errorMessage(e);
    } finally {
      if (id === fileId) fileLoading = false;
    }
  }
  async function mutate(action: () => Promise<unknown>, done?: () => void) {
    saving = true;
    mutationError = '';
    try {
      await action();
      done?.();
      toast('Saved and validated');
      await refresh();
    } catch (e) {
      mutationError = errorMessage(e);
      if (isSignedOut(e)) signedOut();
    } finally {
      saving = false;
    }
  }
  function newTransaction(transaction: Transaction | null = null) {
    selectedTransaction = transaction;
    editorRevision = snapshot?.revision || '';
    mutationError = '';
    transactionOpen = true;
    detailOpen = false;
  }
  function newAccount(name = '') {
    accountClosing = !!name;
    accountName = name;
    accountDate = new Date().toISOString().slice(0, 10);
    accountCurrencies = currency;
    editorRevision = snapshot?.revision || '';
    mutationError = '';
    accountOpen = true;
  }
  function confirm(
    title: string,
    description: string,
    action: () => Promise<void>
  ) {
    confirmTitle = title;
    confirmDescription = description;
    confirmAction = action;
    mutationError = '';
    confirmOpen = true;
  }
  function removeTransaction(t: Transaction) {
    const revision = snapshot?.revision || '';
    confirm(
      'Delete transaction?',
      `${t.payee || t.narration} · ${dateLabel(t.date)}. The previous file version will remain in History.`,
      () =>
        mutate(
          () => api.deleteTransaction({ id: t.id, expectedRevision: revision }),
          () => {
            confirmOpen = false;
            detailOpen = false;
          }
        )
    );
  }
  function restore(entry: HistoryEntry) {
    const revision = snapshot?.revision || '';
    confirm(
      'Restore previous file version?',
      `This restores ${entry.file} to its content before this change. Newer changes in that file will be replaced. The whole ledger must still validate.`,
      () =>
        mutate(
          () =>
            api.restoreVersion({ id: entry.id, expectedRevision: revision }),
          () => (confirmOpen = false)
        )
    );
  }
  async function saveFile() {
    if (!file) return;
    await mutate(
      () =>
        api.writeFile({
          path: file!.path,
          content: draft,
          expectedRevision: file!.revision
        }),
      () => {
        if (file) file = { ...file, content: draft };
      }
    );
  }
  function totalFor(t: Transaction) {
    const positive = t.postings.filter(
      (p) => p.units && new Decimal(p.units.number).isPositive()
    );
    const values: Record<string, Decimal> = {};
    for (const p of positive) {
      const u = p.units!;
      values[u.currency] = (values[u.currency] || new Decimal(0)).plus(
        u.number
      );
    }
    return (
      Object.entries(values)
        .map(([c, n]) => money(n.toString(), c))
        .join(' · ') || '—'
    );
  }
  function accountBalances(name: string) {
    return snapshot?.balances.find((b) => b.account === name)?.positions || [];
  }
  function reportTotal(rows: ReportRow[]) {
    return rows.reduce((v, r) => v.plus(r.number), new Decimal(0)).toString();
  }
  function exportJournal() {
    const rows = [
      [
        'Date',
        'Status',
        'Payee',
        'Description',
        'Account',
        'Amount',
        'Currency',
        'Tags'
      ],
      ...filteredTransactions.flatMap((t) =>
        t.postings.map((p) => [
          t.date,
          t.flag,
          t.payee,
          t.narration,
          p.account,
          p.units?.number || '',
          p.units?.currency || '',
          t.tags.join(' ')
        ])
      )
    ];
    download(
      'transactions.csv',
      rows.map((r) => r.map(csv).join(',')).join('\r\n'),
      'text/csv'
    );
  }
  function exportReport() {
    const rows =
      view === 'income'
        ? report?.incomeStatement
        : view === 'balance'
          ? report?.balanceSheet
          : report?.trialBalance;
    download(
      `${view}-${currency}.csv`,
      [
        ['Account', 'Book value', 'Currency'],
        ...(rows || []).map((r) => [r.account, r.number, currency])
      ]
        .map((r) => r.map(csv).join(','))
        .join('\r\n'),
      'text/csv'
    );
  }
  function toggleTheme() {
    dark = !dark;
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    localStorage.setItem('beancount-theme', dark ? 'dark' : 'light');
  }
  function keydown(event: KeyboardEvent) {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      paletteScope = 'all';
      paletteOpen = !paletteOpen;
    }
    if (
      (event.metaKey || event.ctrlKey) &&
      event.key === 's' &&
      view === 'files'
    ) {
      event.preventDefault();
      if (dirty && !saving) void saveFile();
    }
  }
  beforeNavigate(({ cancel }) => {
    if (dirty && !window.confirm('Leave without saving your file changes?'))
      cancel();
  });
  onMount(() => {
    const narrow = window.matchMedia('(max-width: 800px)');
    const resizeSidebar = () => {
      sidebar = !narrow.matches;
    };
    resizeSidebar();
    narrow.addEventListener('change', resizeSidebar);
    dark = localStorage.getItem('beancount-theme') === 'dark';
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    const reason = new URL(location.href).searchParams.get('auth_error');
    if (reason) {
      loginError =
        reason === 'access_denied'
          ? 'Your account does not have access to this ledger.'
          : 'Sign-in could not be completed. Please try again.';
      historyReplace();
    }
    void start();
    const sessionTimer = setInterval(renewSession, 60_000);
    const checkOnFocus = () => {
      if (document.visibilityState === 'visible') void renewSession();
    };
    document.addEventListener('visibilitychange', checkOnFocus);
    return () => {
      clearInterval(sessionTimer);
      document.removeEventListener('visibilitychange', checkOnFocus);
      narrow.removeEventListener('change', resizeSidebar);
      disposed = true;
      watchController?.abort();
      clearTimeout(toastTimer);
    };
  });
  function historyReplace() {
    const url = new URL(location.href);
    url.searchParams.delete('auth_error');
    window.history.replaceState({}, '', url);
  }
  $effect(() => {
    const a = page.url.searchParams.get('account');
    accountFilter = a || '';
  });
  $effect(() => {
    from;
    to;
    currency;
    if (initialized) void loadReport();
  });
  $effect(() => {
    search;
    from;
    to;
    accountFilter;
    flagFilter;
    pageIndex = 0;
  });
  $effect(() => {
    if (initialized && view === 'history') void loadHistory();
    if (initialized && view === 'files' && !file) {
      const path =
        page.url.searchParams.get('path') || snapshot?.files[0]?.path;
      if (path) void loadFile(path);
    }
  });
</script>

<svelte:head
  ><title
    >{canAccess
      ? `${current?.label || 'Ledger'} · ${snapshot?.title || brandName}`
      : brandName}</title
  ><meta name="description" content="Sign in to your ledger." /></svelte:head
>
<svelte:window
  onkeydown={keydown}
  onbeforeunload={(event) => {
    if (dirty || transactionOpen) {
      event.preventDefault();
      event.returnValue = '';
    }
  }}
/>

{#if !canAccess}
  <div class="login-page">
    <div class="login-card">
      <BrandMark url={session?.brandLogoUrl} />
      <h1>{brandName}</h1>
      <p role={error || loginError ? 'status' : undefined}>
        {loading
          ? 'Checking your session…'
          : error || loginError || 'Sign in to continue.'}
      </p>
      {#if loading}<LoaderCircle
          class="spin"
          size={20}
        />{:else if error && !session}<button class="button" onclick={start}
          >Try again</button
        >{:else}<a class="button" href="/auth/login" data-sveltekit-reload
          >Continue with {session?.provider || 'OpenID Connect'}<ArrowRight
            size={14}
          /></a
        >{/if}
    </div>
  </div>
{:else}
  <div class="app-shell" class:sidebar-collapsed={!sidebar}>
    {#if sidebar}<aside class="sidebar">
        <div class="sidebar-heading">
          <a
            class="workspace-brand"
            href="/"
            title={snapshot?.title || brandName}
          >
            <BrandMark url={session?.brandLogoUrl} small />
            <strong>{snapshot?.title || brandName}</strong>
          </a>
          <button
            class="icon-button"
            aria-label="Hide sidebar"
            onclick={() => (sidebar = false)}
            ><PanelLeftClose size={16} /></button
          >
        </div>
        <button
          class="sidebar-search"
          onclick={() => {
            paletteScope = 'all';
            paletteOpen = true;
          }}
          ><Search size={15} /><span>Search workspace</span><kbd>⌘ K</kbd
          ></button
        >
        <nav class="sidebar-nav" aria-label="Main navigation">
          {#each navGroups as group}
            <div class="nav-group">
              <button
                class="nav-group-heading"
                aria-expanded={!collapsedGroups.includes(group.label)}
                aria-controls={`nav-${group.tone}`}
                onclick={() =>
                  (collapsedGroups = collapsedGroups.includes(group.label)
                    ? collapsedGroups.filter((g) => g !== group.label)
                    : [...collapsedGroups, group.label])}
              >
                <group.icon size={16} class={`nav-tone-${group.tone}`} /><span
                  >{group.label}</span
                ><ChevronRight
                  size={13}
                  class={!collapsedGroups.includes(group.label)
                    ? 'rotated'
                    : ''}
                />
              </button>
              {#if !collapsedGroups.includes(group.label)}<div
                  class="nav-children"
                  id={`nav-${group.tone}`}
                >
                  {#each group.items as item}<a
                      href={item.path === 'overview' ? '/' : `/${item.path}`}
                      class:active={view === item.path}
                      aria-current={view === item.path ? 'page' : undefined}
                    >
                      <item.icon size={15} strokeWidth={1.6} />{item.label}
                      {#if item.path === 'checks' && snapshot?.errors.length}<span
                          class="count danger-count"
                          >{snapshot.errors.length}</span
                        >{/if}
                    </a>{/each}
                </div>{/if}
            </div>
          {/each}
        </nav>
        <button
          class="sidebar-account-search"
          onclick={() => {
            paletteScope = 'accounts';
            paletteOpen = true;
          }}
          ><Wallet size={14} /><span>Find an account</span><Search
            size={12}
          /></button
        >
        <div class="sidebar-bottom">
          <div class="connection">
            <span class:online={connection === 'Live'} class="status-dot"
            ></span>{connection}<span class="muted"
              >{snapshot?.transactions.length || 0} entries</span
            >
          </div>
          <button
            class="user-button"
            aria-label="Workspace settings"
            onclick={() => (settingsOpen = true)}
            ><span class="avatar"
              >{(session?.user?.name || session?.user?.username || 'L')
                .slice(0, 1)
                .toUpperCase()}</span
            ><span
              >{session?.user?.name ||
                session?.user?.username ||
                'Local workspace'}<small
                >{session?.authEnabled
                  ? 'Connected with ' + session.provider
                  : 'Development mode'}</small
              ></span
            ><Settings2 size={15} /></button
          >
        </div>
      </aside>{/if}
    <main class="main" class:files-main={view === 'files'}>
      <header class="topbar">
        <nav class="breadcrumb" aria-label="Breadcrumb">
          {#if !sidebar}<button
              class="icon-button"
              aria-label="Show sidebar"
              onclick={() => (sidebar = true)}
              ><PanelLeftOpen size={17} /></button
            >{/if}
          <a
            class="breadcrumb-workspace"
            href="/"
            title={snapshot?.title || brandName}
            ><BookOpen size={15} strokeWidth={1.5} /><span>Ledger</span></a
          >
          <ChevronRight size={12} /><span
            class="breadcrumb-current"
            aria-current="page">{current?.label || 'Page not found'}</span
          >
        </nav>
        <div class="header-actions">
          <button
            class="icon-button"
            aria-label="Toggle color theme"
            onclick={toggleTheme}
            >{#if dark}<Sun size={16} />{:else}<Moon size={16} />{/if}</button
          ><button
            class="icon-button"
            aria-label="Refresh ledger"
            onclick={refresh}
            disabled={refreshing}
            ><RefreshCw size={15} class={refreshing ? 'spin' : ''} /></button
          ><span class="divider"></span><button
            class="button primary new-transaction-button"
            aria-label="New transaction"
            title="New transaction"
            disabled={!canWrite || !snapshot}
            onclick={() => newTransaction()}
            ><Plus size={14} /><span>New transaction</span></button
          >
        </div>
      </header>
      <div class="content">
        {#if error}<div class="alert error" role="alert">
            <CircleAlert size={16} /><span>{error}</span><button
              class="icon-button"
              aria-label="Dismiss error"
              onclick={() => (error = '')}><X size={14} /></button
            >
          </div>{/if}
        {#if snapshot?.errors.length && view !== 'checks' && view !== 'files'}<a
            href="/checks"
            class="alert warning"
            ><CircleAlert size={16} />{snapshot.errors.length} validation {snapshot
              .errors.length === 1
              ? 'issue'
              : 'issues'} need attention. Reports are paused until they are fixed.<ArrowUpRight
              size={14}
            /></a
          >{/if}
        <div class="page-heading">
          <div>
            <h1>{current?.label || 'Page not found'}</h1>
            <p>
              {view === 'overview'
                ? 'Financial performance and account balances.'
                : view === 'journal'
                  ? 'Review, reconcile, and manage ledger entries.'
                  : view === 'accounts'
                    ? 'Your chart of accounts and current positions.'
                    : view === 'income'
                      ? 'Revenue, expenses, and profit for the selected period.'
                      : view === 'balance'
                        ? 'Assets, liabilities, and equity at the closing date.'
                        : view === 'files'
                          ? 'Your ledger is plain text. Changes are validated before saving.'
                          : view === 'checks'
                            ? 'Keep your books consistent and balanced.'
                            : view === 'history'
                              ? 'A recoverable record of changes made through this workspace.'
                              : 'This page does not exist.'}
            </p>
          </div>
          {#if view === 'journal'}<button class="button" onclick={exportJournal}
              ><Download size={14} />Export CSV</button
            >{:else if view === 'accounts'}<button
              class="button"
              onclick={() => newAccount()}
              disabled={!canWrite || !snapshot}
              ><Plus size={14} />Open account</button
            >{:else if view === 'income' || view === 'balance'}<button
              class="button"
              onclick={exportReport}><Download size={14} />Export CSV</button
            >{/if}
        </div>
        {#if ['overview', 'journal', 'income', 'balance'].includes(view)}<div
            class="filterbar"
          >
            <DateRangeControl bind:from bind:to />
            <span class="report-basis"
              >{view === 'balance' ? 'Closing balances' : 'Ledger reports'}<span
                >USD</span
              ></span
            >
          </div>{/if}
        {#if loading}<div class="empty-state">
            <LoaderCircle class="spin" size={22} />
            <h2>Loading ledger</h2>
            <p>Reading accounts and transactions.</p>
          </div>
        {:else if !snapshot}<div class="empty-state">
            <CircleAlert size={22} />
            <h2>Ledger unavailable</h2>
            <button class="button" onclick={refresh}>Try again</button>
          </div>
        {:else if view === 'overview'}
          <div class="metrics">
            {#each [{ label: 'Assets', value: report?.assets, hint: 'Closing balance · book cost' }, { label: 'Liabilities', value: report?.liabilities, hint: 'Closing balance' }, { label: 'Net assets', value: report?.netWorth, hint: 'Assets less liabilities' }, { label: 'Revenue', value: report?.income, hint: 'During selected period' }, { label: 'Expenses', value: report?.expenses, hint: 'During selected period' }, { label: 'Net profit', value: report?.netIncome, hint: 'Revenue less expenses' }] as metric}<div
                class="metric"
              >
                <span>{metric.label}</span><strong
                  >{report ? money(metric.value, currency) : '—'}</strong
                ><small>{metric.hint}</small>
              </div>{/each}
          </div>
          <div class="chart-grid">
            <section class="panel">
              <div class="panel-heading">
                <h2>Revenue & expenses</h2>
                <div class="legend">
                  <span><i style="background:#8bb9a9"></i>Revenue</span><span
                    ><i style="background:#7774ce"></i>Expenses</span
                  >
                </div>
              </div>
              {#if report?.periods.length}<Chart
                  title="Monthly revenue and expenses"
                  labels={monthLabels}
                  datasets={flowData}
                  {currency}
                />{:else}<div class="chart-empty">
                  No activity in this period
                </div>{/if}
            </section>
            <section class="panel">
              <div class="panel-heading">
                <h2>Net assets</h2>
                <span class="muted small">{currency} · book cost</span>
              </div>
              {#if report?.periods.length}<Chart
                  title="Net assets by month"
                  labels={monthLabels}
                  datasets={worthData}
                  type="line"
                  {currency}
                />{:else}<div class="chart-empty">
                  No balances to display
                </div>{/if}
            </section>
            <section class="panel">
              <div class="panel-heading">
                <div>
                  <h2>Monthly profit</h2>
                  <p class="chart-subtitle">Revenue less expenses</p>
                </div>
                <span class="muted small">USD</span>
              </div>
              {#if report?.periods.length}<Chart
                  title="Monthly net profit"
                  labels={monthLabels}
                  datasets={profitData}
                  {currency}
                />{:else}<div class="chart-empty">
                  No activity in this period
                </div>{/if}
            </section>
            <section class="panel" aria-label="Expenses composition">
              <div class="panel-heading">
                <div>
                  <h2>Expense mix</h2>
                  <p class="chart-subtitle">By account category</p>
                </div>
                <a href="/income" class="text-link"
                  >View report<ArrowUpRight size={13} /></a
                >
              </div>
              <AccountComposition
                rows={report?.incomeStatement || []}
                {currency}
              />
            </section>
            {#each ['Assets', 'Liabilities'] as group}<section
                class="panel"
                aria-label={`${group} composition`}
              >
                <div class="panel-heading">
                  <div>
                    <h2>{group}</h2>
                    <p class="chart-subtitle">
                      Closing balances by account · USD
                    </p>
                  </div>
                  <a href="/balance" class="text-link"
                    >Balance sheet<ArrowUpRight size={13} /></a
                  >
                </div>
                <AccountComposition
                  rows={report?.balanceSheet || []}
                  {currency}
                  group={group === 'Assets' ? 'Assets' : 'Liabilities'}
                />
              </section>{/each}
          </div>
          <div class="overview-bottom">
            <AccountTree accounts={snapshot.accounts} {report} {currency} />
            <section class="panel">
              <div class="panel-heading">
                <h2>Recent transactions</h2>
                <a href="/journal" class="text-link"
                  >View journal<ArrowUpRight size={13} /></a
                >
              </div>
              <div class="recent-list">
                {#each filteredTransactions.slice(0, 6) as tx}<button
                    class="recent-item"
                    onclick={() => {
                      detail = tx;
                      detailOpen = true;
                    }}
                    ><span class="transaction-icon"
                      >{#if tx.postings.some( (p) => p.account.startsWith('Expenses:') )}<ArrowUpRight
                          size={16}
                        />{:else}<ArrowDownLeft size={16} />{/if}</span
                    ><span
                      ><strong>{tx.payee || tx.narration}</strong><small
                        >{dateLabel(tx.date)}{tx.payee
                          ? ' · ' + tx.narration
                          : ''}</small
                      ></span
                    ><span class="number">{totalFor(tx)}</span></button
                  >{:else}<p class="empty-inline">
                    Your first transaction will appear here.
                  </p>{/each}
              </div>
            </section>
          </div>
          <details class="data-disclosure">
            <summary>View monthly figures</summary>
            <div class="table-scroll">
              <table>
                <thead
                  ><tr
                    ><th>Month</th><th class="numeric">Revenue</th><th
                      class="numeric">Expenses</th
                    ><th class="numeric">Net profit</th><th class="numeric"
                      >Net assets</th
                    ></tr
                  ></thead
                ><tbody
                  >{#each report?.periods || [] as p}<tr
                      ><td>{p.month}</td><td class="numeric"
                        >{money(p.income, currency)}</td
                      ><td class="numeric">{money(p.expenses, currency)}</td><td
                        class="numeric">{money(p.netIncome, currency)}</td
                      ><td class="numeric">{money(p.netWorth, currency)}</td
                      ></tr
                    >{/each}</tbody
                >
              </table>
            </div>
          </details>
        {:else if view === 'journal'}
          <div class="journal-toolbar">
            <label class="search-field"
              ><Search size={15} /><input
                aria-label="Search transactions"
                bind:value={search}
                placeholder="Search payee, description, account, or tag…"
              /></label
            ><AccountPicker
              label="Filter by account"
              accounts={accountPaths(snapshot.accounts)}
              bind:value={accountFilter}
              placeholder="All accounts"
              allowEmpty
            /><select aria-label="Transaction status" bind:value={flagFilter}
              ><option value="">All statuses</option><option value="*"
                >Cleared</option
              ><option value="!">Pending</option></select
            >
          </div>
          <div class="table-wrap">
            <div class="table-scroll">
              <table class="journal-table">
                <thead
                  ><tr
                    ><th class="status-cell"></th><th>Date</th><th
                      >Payee / description</th
                    ><th>Accounts</th><th class="numeric">Amount</th><th
                    ></th></tr
                  ></thead
                ><tbody
                  >{#each transactions as tx}<tr
                      ><td class="status-cell"
                        ><span title={tx.flag === '*' ? 'Cleared' : 'Pending'}
                          >{#if tx.flag === '*'}<CircleCheck
                              size={14}
                              class="muted"
                            />{:else}<Circle
                              size={14}
                              class="pending"
                            />{/if}</span
                        ></td
                      ><td class="nowrap muted">{dateLabel(tx.date)}</td><td
                        ><button
                          class="row-title"
                          onclick={() => {
                            detail = tx;
                            detailOpen = true;
                          }}>{tx.payee || tx.narration}</button
                        >{#if tx.payee}<div class="cell-subtitle">
                            {tx.narration}
                          </div>{/if}{#if tx.tags.length}<div class="tags">
                            {#each tx.tags as tag}<button
                                onclick={() => (search = tag)}>#{tag}</button
                              >{/each}
                          </div>{/if}</td
                      ><td class="account-cell"
                        >{tx.postings
                          .slice(0, 2)
                          .map((p) => shortAccount(p.account))
                          .join(' → ')}{tx.postings.length > 2
                          ? ' +' + (tx.postings.length - 2)
                          : ''}</td
                      ><td class="numeric nowrap">{totalFor(tx)}</td><td
                        ><button
                          class="icon-button"
                          aria-label={`Edit ${tx.payee || tx.narration}`}
                          disabled={!canWrite}
                          onclick={() => newTransaction(tx)}
                          ><Pencil size={14} /></button
                        ></td
                      ></tr
                    >{:else}<tr
                      ><td colspan="6"
                        ><div class="empty-state compact">
                          <Search size={22} />
                          <h2>No transactions found</h2>
                          <p>Adjust your filters or add a transaction.</p>
                        </div></td
                      ></tr
                    >{/each}</tbody
                >
              </table>
            </div>
            <div class="table-footer">
              <span
                >{filteredTransactions.length} transactions{filteredTransactions.length
                  ? ' · ' +
                    (pageIndex * 50 + 1) +
                    '–' +
                    Math.min((pageIndex + 1) * 50, filteredTransactions.length)
                  : ''}</span
              >
              <div>
                <button
                  class="button small"
                  disabled={pageIndex === 0}
                  onclick={() => pageIndex--}>Previous</button
                ><button
                  class="button small"
                  disabled={(pageIndex + 1) * 50 >= filteredTransactions.length}
                  onclick={() => pageIndex++}>Next</button
                >
              </div>
            </div>
          </div>
        {:else if view === 'accounts'}
          <div class="journal-toolbar">
            <label class="search-field"
              ><Search size={15} /><input
                aria-label="Search accounts"
                bind:value={accountSearch}
                placeholder="Find an account…"
              /></label
            ><select aria-label="Account type" bind:value={accountKind}
              >{#each ['All', 'Assets', 'Liabilities', 'Equity', 'Income', 'Expenses'] as kind}<option
                  >{kind}</option
                >{/each}</select
            >
          </div>
          <div class="table-wrap">
            <div class="table-scroll">
              <table>
                <thead
                  ><tr
                    ><th>Account</th><th>Currencies</th><th>Opened</th><th
                      class="numeric">Current positions</th
                    ><th>Status</th><th></th></tr
                  ></thead
                ><tbody
                  >{#each visibleAccounts as a}<tr
                      ><td
                        ><a
                          class="account-link"
                          href={`/journal?account=${encodeURIComponent(a.name)}`}
                          ><span class="account-type"
                            >{a.name.split(':')[0]}</span
                          >{shortAccount(a.name)}</a
                        ></td
                      ><td>{a.currencies.join(', ') || 'Unrestricted'}</td><td
                        class="muted nowrap">{dateLabel(a.opened)}</td
                      ><td class="numeric"
                        >{#each accountBalances(a.name) as p}<div>
                            {money(
                              p.units?.number,
                              p.units?.currency
                            )}{#if p.cost}<small class="cell-subtitle"
                                >{p.cost}</small
                              >{/if}
                          </div>{:else}<span class="muted">0.00</span
                          >{/each}</td
                      ><td
                        ><span class="badge" class:subtle={!!a.closed}
                          >{a.closed ? 'Closed' : 'Open'}</span
                        ></td
                      ><td
                        >{#if !a.closed}<button
                            class="button ghost small"
                            disabled={!canWrite}
                            onclick={() => newAccount(a.name)}>Close</button
                          >{/if}</td
                      ></tr
                    >{:else}<tr
                      ><td colspan="6" class="empty-inline"
                        >No matching accounts.</td
                      ></tr
                    >{/each}</tbody
                >
              </table>
            </div>
            <div class="table-footer">
              {visibleAccounts.length} accounts · Positions retain their original
              currencies and cost lots.
            </div>
          </div>
        {:else if view === 'income' || view === 'balance'}
          {#if !report}<div class="empty-state">
              <CircleAlert size={22} />
              <h2>Report unavailable</h2>
              <p>Resolve validation errors or adjust your date range.</p>
            </div>{:else}
            <div class="report-layout">
              <div class="panel statement">
                <div class="panel-heading">
                  <h2>
                    {view === 'income' ? 'Income statement' : 'Balance sheet'}
                  </h2>
                  <span class="muted small">{currency}</span>
                </div>
                {#each view === 'income' ? ['Income', 'Expenses'] : ['Assets', 'Liabilities', 'Equity'] as group}{@const rows =
                    (
                      view === 'income'
                        ? report.incomeStatement
                        : report.balanceSheet
                    ).filter((r) =>
                      r.account.startsWith(group + ':')
                    )}{@const reverse = [
                    'Income',
                    'Liabilities',
                    'Equity'
                  ].includes(group)}
                  <div class="statement-group">
                    <h3>{group === 'Income' ? 'Revenue' : group}</h3>
                    {#each rows as row}<a
                        class="statement-row"
                        href={`/journal?account=${encodeURIComponent(row.account)}`}
                        ><span>{shortAccount(row.account)}</span><span
                          class="number"
                          >{money(
                            new Decimal(row.number)
                              .mul(reverse ? -1 : 1)
                              .toString(),
                            currency
                          )}</span
                        ></a
                      >{:else}<div class="statement-row muted">
                        <span
                          >No {group === 'Income'
                            ? 'revenue'
                            : group.toLowerCase()} entries</span
                        ><span>{money(0, currency)}</span>
                      </div>{/each}
                    <div class="statement-row statement-total">
                      <span
                        >Total {group === 'Income'
                          ? 'revenue'
                          : group.toLowerCase()}</span
                      ><strong
                        >{money(
                          new Decimal(reportTotal(rows))
                            .mul(reverse ? -1 : 1)
                            .toString(),
                          currency
                        )}</strong
                      >
                    </div>
                  </div>{/each}
                {#if view === 'balance'}<div class="statement-row earnings-row">
                    <span
                      >Accumulated earnings <small class="muted"
                        >Revenue less expenses through end date</small
                      ></span
                    ><strong
                      >{money(
                        new Decimal(report.netWorth)
                          .minus(report.equity)
                          .toString(),
                        currency
                      )}</strong
                    >
                  </div>{/if}
                <div class="statement-result">
                  <span>{view === 'income' ? 'Net profit' : 'Net assets'}</span
                  ><strong
                    >{money(
                      view === 'income' ? report.netIncome : report.netWorth,
                      currency
                    )}</strong
                  >
                </div>
              </div>
              <div class="report-side">
                <section class="panel">
                  <div class="panel-heading">
                    <h2>
                      {view === 'income'
                        ? 'Monthly activity'
                        : 'Net assets over time'}
                    </h2>
                  </div>
                  <Chart
                    title={view === 'income'
                      ? 'Monthly revenue and expenses'
                      : 'Net assets over time'}
                    labels={monthLabels}
                    datasets={view === 'income' ? flowData : worthData}
                    type={view === 'income' ? 'bar' : 'line'}
                    {currency}
                  />
                </section>
                <p class="report-note">
                  Amounts use recorded book cost in {currency}. Other currencies
                  remain separate; market prices and exchange rates are not
                  applied.
                </p>
              </div>
            </div>
            <details class="data-disclosure">
              <summary>Trial balance</summary>
              <div class="table-scroll">
                <table>
                  <thead
                    ><tr
                      ><th>Account</th><th class="numeric">Debit</th><th
                        class="numeric">Credit</th
                      ></tr
                    ></thead
                  ><tbody
                    >{#each report.trialBalance as r}<tr
                        ><td>{r.account}</td><td class="numeric"
                          >{new Decimal(r.number).isPositive()
                            ? money(r.number, currency)
                            : '—'}</td
                        ><td class="numeric"
                          >{new Decimal(r.number).isNegative()
                            ? money(
                                new Decimal(r.number).negated().toString(),
                                currency
                              )
                            : '—'}</td
                        ></tr
                      >{/each}<tr class="total-row"
                      ><td>Net balance</td><td colspan="2" class="numeric"
                        >{money(reportTotal(report.trialBalance), currency)}</td
                      ></tr
                    ></tbody
                  >
                </table>
              </div>
            </details>
          {/if}
        {:else if view === 'files'}
          <div class="files-layout">
            <nav class="file-list" aria-label="Ledger files">
              <div class="file-list-heading">
                Files <span>{snapshot.files.length}</span>
              </div>
              {#each snapshot.files as f}<button
                  class:active={file?.path === f.path}
                  aria-current={file?.path === f.path ? 'true' : undefined}
                  title={f.path}
                  onclick={() => loadFile(f.path)}
                  ><Files size={14} /><span>{f.path}</span><small
                    >{f.bytes} B</small
                  ></button
                >{/each}
            </nav>
            <section class="file-editor">
              <div class="file-heading">
                <span
                  >{file?.path || 'Select a file'}{#if dirty}<span
                      class="unsaved">Unsaved</span
                    >{/if}</span
                >
                <div>
                  <button
                    class="icon-button"
                    aria-label="Download file"
                    disabled={!file}
                    onclick={() =>
                      file &&
                      download(
                        file.path.split('/').pop() || 'ledger.beancount',
                        draft
                      )}><Download size={15} /></button
                  ><button
                    class="button primary small"
                    disabled={!canWrite || !dirty || saving || fileLoading}
                    onclick={saveFile}
                    >{saving ? 'Validating…' : 'Save changes'}</button
                  >
                </div>
              </div>
              {#if fileError || mutationError}<div
                  class="alert error"
                  role="alert"
                >
                  {fileError || mutationError}
                </div>{/if}{#if file && file.revision !== snapshot.revision && dirty}<div
                  class="alert warning"
                >
                  The ledger changed since you opened this file. Copy your draft
                  before reloading; saving will reject a stale revision.<button
                    class="button small"
                    onclick={() => {
                      if (
                        window.confirm(
                          'Discard this draft and load the latest file?'
                        )
                      ) {
                        draft = file!.content;
                        void loadFile(file!.path);
                      }
                    }}>Reload</button
                  >
                </div>{/if}{#if fileLoading}<div class="empty-state">
                  <LoaderCircle class="spin" size={20} />
                </div>{:else if file}{#key file.path}
                  {#await import('./components/LedgerEditor.svelte')}
                    <div class="empty-state">
                      <LoaderCircle class="spin" size={20} />
                    </div>
                  {:then { default: LedgerEditor }}
                    <LedgerEditor bind:value={draft} readonly={!canWrite} />
                  {/await}
                {/key}
                <div class="file-status">
                  <span
                    >{draft.split('\n').length} lines · UTF-8 · Beancount</span
                  ><span>⌘ / Ctrl S to save</span>
                </div>{:else}<div class="empty-state">
                  <Files size={22} />
                  <p>Select a ledger file.</p>
                </div>{/if}
            </section>
          </div>
        {:else if view === 'checks'}
          <section
            class="validation-summary"
            class:has-errors={snapshot.errors.length > 0}
          >
            {#if snapshot.errors.length}<CircleAlert size={24} />
              <div>
                <h2>
                  {snapshot.errors.length} validation {snapshot.errors
                    .length === 1
                    ? 'issue'
                    : 'issues'}
                </h2>
                <p>
                  Open the source file to resolve each issue. Invalid edits
                  cannot be saved through this app.
                </p>
              </div>{:else}<CircleCheck size={24} />
              <div>
                <h2>Your ledger is balanced</h2>
                <p>
                  All {snapshot.transactions.length} transactions and {snapshot
                    .accounts.length} accounts passed Beancount validation across
                  {snapshot.files.length} files.
                </p>
              </div>{/if}<button
              class="button"
              onclick={refresh}
              disabled={refreshing}>Run checks</button
            >
          </section>
          {#if snapshot.diagnostics.length}<div class="table-wrap">
              <table>
                <thead><tr><th>Source</th><th>Issue</th><th></th></tr></thead
                ><tbody
                  >{#each snapshot.diagnostics as issue}<tr
                      ><td class="mono">{issue.file}:{issue.line}</td><td
                        >{issue.message}</td
                      ><td
                        ><button
                          class="button small"
                          onclick={async () => {
                            file = undefined;
                            await goto(
                              '/files?path=' + encodeURIComponent(issue.file)
                            );
                          }}>Open file<ArrowUpRight size={13} /></button
                        ></td
                      ></tr
                    >{/each}</tbody
                >
              </table>
            </div>{/if}
          <div class="panel ledger-facts">
            <h2>Ledger details</h2>
            <dl>
              <div>
                <dt>Accounts</dt>
                <dd>{snapshot.accounts.length}</dd>
              </div>
              <div>
                <dt>Transactions</dt>
                <dd>{snapshot.transactions.length}</dd>
              </div>
              <div>
                <dt>Currencies</dt>
                <dd>{snapshot.currencies.join(', ') || 'None declared'}</dd>
              </div>
              <div>
                <dt>Files</dt>
                <dd>{snapshot.files.length}</dd>
              </div>
              <div>
                <dt>Revision</dt>
                <dd class="mono">{snapshot.revision.slice(0, 12)}</dd>
              </div>
            </dl>
          </div>
        {:else if view === 'history'}
          <div class="table-wrap">
            <div class="table-scroll">
              <table>
                <thead
                  ><tr
                    ><th>When</th><th>Change</th><th>File</th><th>Actor</th><th
                    ></th></tr
                  ></thead
                ><tbody
                  >{#each history as entry}<tr
                      ><td class="nowrap"
                        >{new Date(entry.timestamp).toLocaleString()}</td
                      ><td>{entry.operation.replaceAll('_', ' ')}</td><td
                        class="mono">{entry.file}</td
                      ><td class="muted actor-cell" title={entry.actor}
                        >{entry.actor}</td
                      ><td
                        ><button
                          class="button small"
                          disabled={!canWrite}
                          onclick={() => restore(entry)}
                          ><Undo2 size={13} />Restore before</button
                        ></td
                      ></tr
                    >{:else}<tr
                      ><td colspan="5"
                        ><div class="empty-state compact">
                          <HistoryIcon size={22} />
                          <h2>No changes yet</h2>
                          <p>
                            Edits made through the UI or MCP will appear here
                            with a recoverable file version. External edits are
                            detected live but are not recorded here.
                          </p>
                        </div></td
                      ></tr
                    >{/each}</tbody
                >
              </table>
            </div>
            <div class="table-footer">
              Latest {history.length} changes · Recovery records stay on the ledger
              volume.
            </div>
          </div>
        {:else}<div class="empty-state">
            <h2>Page not found</h2>
            <a href="/" class="button">Back to overview</a>
          </div>{/if}
        <footer class="content-footer">
          <span
            ><span class="status-dot" class:online={!snapshot?.errors.length}
            ></span>{snapshot?.errors.length
              ? 'Needs attention'
              : 'Ledger validated'}</span
          ><span>{brandName} · {snapshot?.revision.slice(0, 8) || '—'}</span>
        </footer>
      </div>
    </main>
  </div>
{/if}

{#if notice}<div class="toast" role="status">
    <Check size={15} />{notice}
  </div>{/if}
{#if snapshot}<TransactionEditor
    bind:open={transactionOpen}
    transaction={selectedTransaction}
    accounts={snapshot.accounts}
    files={snapshot.files}
    revision={snapshot.revision}
    {saving}
    error={mutationError}
    conflict={staleEditor}
    save={(value) =>
      mutate(
        () => api.saveTransaction(value),
        () => (transactionOpen = false)
      )}
  />{/if}
<Modal
  bind:open={detailOpen}
  title={detail?.payee || 'Transaction'}
  description={detail
    ? dateLabel(detail.date) +
      ' · ' +
      (detail.flag === '*' ? 'Cleared' : 'Pending')
    : ''}
  wide
>
  {#if detail}<p class="transaction-narration">{detail.narration}</p>
    <table>
      <thead><tr><th>Account</th><th class="numeric">Amount</th></tr></thead
      ><tbody
        >{#each detail.postings as p}<tr
            ><td
              ><a
                href={`/journal?account=${encodeURIComponent(p.account)}`}
                onclick={() => (detailOpen = false)}>{p.account}</a
              >{#if p.cost}<small class="cell-subtitle">Cost: {p.cost}</small
                >{/if}{#if p.price}<small class="cell-subtitle"
                  >Price: {money(p.price.number, p.price.currency)}</small
                >{/if}</td
            ><td class="numeric">{money(p.units?.number, p.units?.currency)}</td
            ></tr
          >{/each}</tbody
      >
    </table>
    <div class="source-caption">{detail.file}:{detail.line}</div>
    <pre class="source-preview">{detail.source}</pre>
    <div class="modal-footer">
      <button
        class="button danger"
        disabled={!canWrite}
        onclick={() => detail && removeTransaction(detail)}
        ><Trash2 size={14} />Delete</button
      ><button
        class="button"
        onclick={() => {
          void navigator.clipboard
            .writeText(detail!.source)
            .then(() => toast('Transaction copied'));
        }}><Copy size={14} />Copy source</button
      ><button
        class="button primary"
        disabled={!canWrite}
        onclick={() => newTransaction(detail)}
        ><Pencil size={14} />Edit transaction</button
      >
    </div>{/if}
</Modal>
<Modal
  bind:open={accountOpen}
  title={accountClosing ? 'Close account' : 'Open account'}
  description={accountClosing
    ? 'The account must have a zero balance and no later postings.'
    : 'Create an account with explicit currency restrictions.'}
>
  <form
    onsubmit={(e) => {
      e.preventDefault();
      void mutate(
        () =>
          accountClosing
            ? api.closeAccount({
                name: accountName,
                date: accountDate,
                expectedRevision: editorRevision
              })
            : api.openAccount({
                name: accountName,
                date: accountDate,
                currencies: accountCurrencies
                  .split(',')
                  .map((c) => c.trim())
                  .filter(Boolean),
                expectedRevision: editorRevision
              }),
        () => (accountOpen = false)
      );
    }}
  >
    {#if mutationError}<div class="alert error" role="alert">
        {mutationError}
      </div>{/if}<label
      >Account name<input
        bind:value={accountName}
        placeholder="Expenses:Software"
        required
        readonly={accountClosing}
      /></label
    >
    <div class="field-label account-date-field">
      <span>{accountClosing ? 'Closing date' : 'Opening date'}</span
      ><DateControl
        bind:value={accountDate}
        label={accountClosing ? 'Closing date' : 'Opening date'}
      />
    </div>
    {#if !accountClosing}<label
        >Currencies<input
          bind:value={accountCurrencies}
          placeholder="USD, EUR"
          required
        /><small class="muted">Separate multiple currencies with commas.</small
        ></label
      >{/if}
    <div class="modal-footer">
      <button class="button" type="button" onclick={() => (accountOpen = false)}
        >Cancel</button
      ><button
        class="button primary"
        disabled={saving || editorRevision !== snapshot?.revision}
        >{saving
          ? 'Validating…'
          : accountClosing
            ? 'Close account'
            : 'Open account'}</button
      >
    </div>
    {#if editorRevision !== snapshot?.revision}<p class="alert warning">
        The ledger changed. Reopen this dialog to use the latest revision.
      </p>{/if}
  </form>
</Modal>
<Modal
  bind:open={confirmOpen}
  title={confirmTitle}
  description={confirmDescription}
  >{#if mutationError}<div class="alert error" role="alert">
      {mutationError}
    </div>{/if}
  <div class="modal-footer">
    <button class="button" onclick={() => (confirmOpen = false)}>Cancel</button
    ><button class="button danger" disabled={saving} onclick={confirmAction}
      >{saving ? 'Validating…' : 'Confirm'}</button
    >
  </div></Modal
>
<Modal
  bind:open={settingsOpen}
  title="Your workspace"
  description="Session and connection details."
  ><dl class="settings-list">
    <div>
      <dt>Signed in as</dt>
      <dd>{session?.user?.name || session?.user?.username || 'Development'}</dd>
    </div>
    <div>
      <dt>Email</dt>
      <dd>{session?.user?.email || '—'}</dd>
    </div>
    <div>
      <dt>Provider</dt>
      <dd>
        {session?.authEnabled ? session.provider : 'Authentication disabled'}
      </dd>
    </div>
    <div>
      <dt>Groups</dt>
      <dd>
        {#if session?.user?.groups.length}
          <ul class="workspace-groups" aria-label="Your groups">
            {#each session.user.groups as group}<li>{group}</li>{/each}
          </ul>
        {:else}No groups provided{/if}
      </dd>
    </div>
  </dl>
  <div class="modal-footer">
    {#if session?.authEnabled}<form method="POST" action="/auth/logout">
        <button class="button" type="submit"
          ><LogOut size={14} />Sign out</button
        >
      </form>{/if}<button class="button" onclick={toggleTheme}
      >{dark ? 'Use light theme' : 'Use dark theme'}</button
    >
  </div></Modal
>
<CommandPalette
  bind:open={paletteOpen}
  accounts={snapshot?.accounts || []}
  transactions={snapshot?.transactions || []}
  {nav}
  initialScope={paletteScope}
  onTransaction={(tx) => {
    detail = tx;
    detailOpen = true;
  }}
/>
