<script lang="ts">
  import { onMount } from 'svelte';
  import { basicSetup } from 'codemirror';
  import { Compartment, EditorState, Transaction } from '@codemirror/state';
  import { EditorView } from '@codemirror/view';
  import { beancountLanguage } from '../beancount-language';

  let { value = $bindable(''), readonly = false } = $props<{
    value: string;
    readonly?: boolean;
  }>();
  let host: HTMLDivElement;
  let editor = $state.raw<EditorView>();
  const access = new Compartment();

  onMount(() => {
    const view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          beancountLanguage,
          EditorView.cspNonce.of(
            document.querySelector<HTMLMetaElement>('meta[name="style-nonce"]')
              ?.content || ''
          ),
          EditorState.tabSize.of(2),
          access.of(EditorState.readOnly.of(readonly)),
          EditorView.contentAttributes.of({
            'aria-label': 'Ledger file content',
            'aria-multiline': 'true',
            spellcheck: 'false',
            autocapitalize: 'off'
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) value = update.state.doc.toString();
          })
        ]
      })
    });
    editor = view;
    return () => view.destroy();
  });

  $effect(() => {
    if (editor && editor.state.doc.toString() !== value) {
      editor.dispatch({
        changes: { from: 0, to: editor.state.doc.length, insert: value },
        annotations: Transaction.addToHistory.of(false)
      });
    }
  });
  $effect(() => {
    editor?.dispatch({
      effects: access.reconfigure(EditorState.readOnly.of(readonly))
    });
  });
</script>

<div class="ledger-code-editor" bind:this={host}></div>
