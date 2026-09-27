import {
  HighlightStyle,
  StreamLanguage,
  syntaxHighlighting
} from '@codemirror/language';
import { tags } from '@lezer/highlight';

// A tolerant lexer: unfinished or invalid drafts must still be editable.
const language = StreamLanguage.define({
  startState: () => ({ quoted: false }),
  token(stream, state) {
    if (!state.quoted && stream.eatSpace()) return null;
    if (!state.quoted && stream.peek() === ';') {
      stream.skipToEnd();
      return 'comment';
    }
    if (state.quoted || stream.peek() === '"') {
      if (!state.quoted) stream.next();
      state.quoted = true;
      while (!stream.eol()) {
        const char = stream.next();
        if (char === '\\') stream.next();
        else if (char === '"') {
          state.quoted = false;
          break;
        }
      }
      return 'string';
    }
    if (stream.match(/^\d{4}[-/]\d{2}[-/]\d{2}\b/)) return 'atom';
    if (
      stream.match(
        /^(?:Assets|Liabilities|Equity|Income|Expenses)(?::[\p{L}\p{N}-]+)+/u
      )
    )
      return 'variableName';
    if (
      stream.match(
        /^(?:open|close|commodity|pad|balance|price|event|query|note|document|custom|txn|option|include|plugin|pushtag|poptag|pushmeta|popmeta)\b/
      )
    )
      return 'keyword';
    if (stream.match(/^[a-z][A-Za-z0-9_-]*(?=:)/)) return 'propertyName';
    if (stream.match(/^[#^][A-Za-z0-9_./-]+/)) return 'labelName';
    if (stream.match(/^[+-]?(?:\d[\d,]*(?:\.\d*)?|\.\d+)\b/)) return 'number';
    if (stream.match(/^[A-Z][A-Z0-9._'-]*\b/)) return 'typeName';
    if (stream.match(/^[*!&#?%PSTCUR]/)) return 'operator';
    stream.next();
    return null;
  },
  languageData: { commentTokens: { line: ';' } }
});

export const beancountLanguage = [
  language,
  syntaxHighlighting(
    HighlightStyle.define([
      { tag: tags.comment, class: 'syntax-comment' },
      { tag: tags.string, class: 'syntax-string' },
      { tag: tags.atom, class: 'syntax-date' },
      { tag: tags.variableName, class: 'syntax-account' },
      { tag: tags.keyword, class: 'syntax-keyword' },
      { tag: tags.propertyName, class: 'syntax-keyword' },
      { tag: tags.labelName, class: 'syntax-tag' },
      { tag: tags.number, class: 'syntax-number' },
      { tag: tags.typeName, class: 'syntax-currency' },
      { tag: tags.operator, class: 'syntax-keyword' }
    ])
  )
];
