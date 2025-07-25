import { EditorView, basicSetup } from "codemirror";
import { StreamLanguage, indentUnit } from "@codemirror/language";
import { Decoration, WidgetType, lineNumbers, keymap } from "@codemirror/view";
import { Compartment, EditorState, StateEffect, StateField } from "@codemirror/state";
import { defaultKeymap, indentMore, indentLess } from "@codemirror/commands";
import { json, jsonParseLinter } from "@codemirror/lang-json"
import { githubLight } from '@uiw/codemirror-theme-github';
import {
  linter,
  lintGutter,
  setDiagnostics,
} from "@codemirror/lint";
import { rego } from "./lib/codemirror/legacy-modes/rego.js";

// Compartments
const wrappingCompartment = new Compartment();

// Effects
const setCoverageLines = StateEffect.define({
  map: (value, change) => value // No mapping needed, coverage is discarded on doc change
});
const resetCoverageLines = StateEffect.define();
const setHighlightedLine = StateEffect.define({
  map: (value, mapping) => value // No mapping needed for line numbers
});
const setStatusBar = StateEffect.define();

// Decorations
const coveredLineMark = Decoration.line({ class: "line-covered" });
const notCoveredLineMark = Decoration.line({ class: "line-not-covered" });

// Helper: builds an extension that updates the button
function selectionListener(evalButtonText) {
  return EditorView.updateListener.of(update => {
    if (update.selectionSet) {
      const selected = update.state.selection.main.from !== update.state.selection.main.to;
      evalButtonText.textContent = selected ? "Evaluate Selection" : "Evaluate";
    }
  });
}

// The field stores covered lines as a Set of 1-based line numbers (or null = no coverage)
const coverageStateField = StateField.define({
  create() {
    return Decoration.none;
  },
  update(decorations, tr) {
    // Remove all coverage marks if the document changed
    if (tr.docChanged) return Decoration.none;
    let newCoverageLines = null;
    for (let e of tr.effects) {
      if (e.is(setCoverageLines)) {
        newCoverageLines = new Set(e.value.lines);
      }
      if (e.is(resetCoverageLines)) {
        newCoverageLines = null;
      }
    }
    // If no new coverage, keep previous decorations
    if (!newCoverageLines) return decorations;

    // Otherwise, compute the decorations anew
    const doc = tr.state.doc;
    const decos = [];
    for (let i = 1; i <= doc.lines; i++) {
      const line = doc.line(i);
      if (shouldIgnoreLine(line.text)) continue;
      if (newCoverageLines.has(i)) {
        decos.push(coveredLineMark.range(line.from));
      } else {
        decos.push(notCoveredLineMark.range(line.from));
      }
    }
    return Decoration.set(decos, true);
  },
  provide: field => EditorView.decorations.from(field)
});

const highlightedLineField = StateField.define({
  create() { return Decoration.none; },
  update(deco, tr) {
    for (let e of tr.effects) {
      if (e.is(setHighlightedLine)) {
        if (e.value == null || isNaN(e.value)) return Decoration.none;
        const line = tr.state.doc.line(e.value);
        if (!line) return Decoration.none;
        return Decoration.set([
          Decoration.line({ class: "line-highlight" }).range(line.from)
        ]);
      }
    }
    // Remove decoration on doc change unless still present
    if (tr.docChanged) return Decoration.none;
    return deco;
  },
  provide: f => EditorView.decorations.from(f)
});

// WidgetType implementation for the status bar
class StatusBarWidget extends WidgetType {
  constructor(text) {
    super();
    this.text = text;
  }
  toDOM() {
    const div = document.createElement("div");
    div.className = "cm-statusBar";
    div.textContent = this.text;
    return div;
  }
}

// StateField to manage the status bar widget
const statusBarField = StateField.define({
  create() {
    return Decoration.none;
  },
  update(deco, tr) {
    for (let e of tr.effects) {
      if (e.is(setStatusBar)) {
        const decoSet = e.value
          ? Decoration.set([
              Decoration.widget({
                widget: new StatusBarWidget(e.value),
                side: -1, // insert before line 1
                block: true
              }).range(0)
            ])
          : Decoration.none;
        return decoSet;
      }
    }
    return deco;
  },
  provide: f => EditorView.decorations.from(f)
});

const safeJsonLinter = linter(view => {
  const doc = view.state.doc.toString().trim();
  if (doc === "") return [];
  return jsonParseLinter()(view);
});

export class CodeMirrorWrapper {
  constructor({
     parent,
     lang = 'json',
     initialValue = "",
     onDocChanged,
     onLineNumberClick,
     onModEnter,
     evalButtonText,
     formatHandler,
     readOnly = false,
    } = {}) {
    this.currentDiagnostics = [];
    const persistentLinter = linter(() => this.currentDiagnostics);
    this.formatHandler = formatHandler;
    this.parent = parent;
    this.extensions = [
      indentUnit.of("\t"),
      basicSetup,
      githubLight,
      keymap.of([
        ...defaultKeymap,
        {
          key: "Tab",
          preventDefault: true,
          run: indentMore,
        },
        {
          key: "Shift-Tab",
          preventDefault: true,
          run: indentLess,
        },
      ]),
    ];

    this.onDocChanged = typeof onDocChanged === "function"
      ? debounce(onDocChanged, 300 /* ms */)
      : undefined;

    const docChangeListener = EditorView.updateListener.of((update) => {
      if (update.docChanged) this.clearDiagnostics();
      if (update.docChanged && typeof this.onDocChanged === "function") {
        this.onDocChanged();
      }
    });

    if (evalButtonText) {
      this.extensions.push(selectionListener(evalButtonText));
    }

    if (lang === 'rego' && onLineNumberClick) {
      const lineNumbersExt = lineNumbers({
        domEventHandlers: {
          click: (view, line, event) => {
            const lineNumber = view.state.doc.lineAt(line.from).number;
            onLineNumberClick(lineNumber);
            return true; // Prevents default if you handled it
          }
        }
      });
      this.extensions.push(lineNumbersExt);
    }

    if (onModEnter) {
      this.extensions.unshift(keymap.of([ // NOTE(sr): this keymap must come before basicSetup
        {
          key: "Mod-Enter",
          run: () => { onModEnter(); return true; }
        }
      ]));
    }

    this.editor = new EditorView({
      state: EditorState.create({
        doc: initialValue,
        extensions: lang === 'rego' ?
          [...this.extensions,
           wrappingCompartment.of([]),
           StreamLanguage.define(rego),
           coverageStateField,
           highlightedLineField,
           persistentLinter,
           lintGutter(),
           docChangeListener,
           statusBarField,
         ] : [...this.extensions,
           wrappingCompartment.of([]),
           statusBarField,
           json(),
           lintGutter(),
           safeJsonLinter,
           EditorState.readOnly.of(readOnly),
         ]
      }),
      parent: this.parent,
    });
  }

  setValue(newValue) {
    this.editor.dispatch({
      changes: {
        from: 0,
        to: this.editor.state.doc.length,
        insert: newValue,
      }
    });
  }

  getValue() {
    return this.editor.state.doc.toString();
  }

  getSelection() {
    const sel = this.editor.state.selection.main;
    return this.editor.state.doc.slice(sel.from, sel.to).toString();
  }

  focus() {
    this.editor.focus();
  }

  setCoverage(lines) {
    this.editor.dispatch({
      effects: setCoverageLines.of({ lines })
    });
  }

  resetCoverage() {
    this.editor.dispatch({
      effects: resetCoverageLines.of()
    });
  }

  setDiagnostics(diagnosticsArray) {
    const apply = this.formatHandler;
    this.currentDiagnostics = diagnosticsFromRegalJSON(diagnosticsArray, this.editor.state.doc).map(d => ({
      ...d,
      actions: d.source == 'style/opa-fmt' ? [{name: "format", apply}, ...d.actions] : d.actions,
    }));
    this.editor.dispatch({ effects: [] }); // Triggers re-lint
  }

  clearDiagnostics() {
    this.currentDiagnostics = [];
    this.editor.dispatch({ effects: [] }); // Triggers re-lint
  }

  highlightLine(lineNumber) {
    this.editor.dispatch({
      effects: setHighlightedLine.of(lineNumber)
    });
  }

  clearHighlightedLine() {
    this.editor.dispatch({
      effects: setHighlightedLine.of(null)
    });
  }

  scrollIntoView(lineNumber) {
    if (typeof lineNumber === 'number' && lineNumber > 0) {
      const line = this.editor.state.doc.line(lineNumber);
      if (line) {
        this.editor.dispatch({
          effects: EditorView.scrollIntoView(line.from, { y: 'center' })
        });
      }
    }
  }

  setLineWrapping(enabled) {
    this.editor.dispatch({
      effects: wrappingCompartment.reconfigure(
        enabled ? EditorView.lineWrapping : []
      )
    });
  }

  setStatusBar(text) {
    this.editor.dispatch({
      effects: setStatusBar.of(text)
    });
  }
}

function diagnosticsFromRegalJSON(jsonDiagnostics, doc) {
  // Supports both Regal JSON diagnostics and rego_parse_error format
  const diagnostics = [];
  for (const diag of jsonDiagnostics) {
    // Detect rego_parse_error format
    if (diag.code === "rego_parse_error" && diag.location && diag.location.row) {
      const line = doc.line(diag.location.row);
      const { to, from } = line;
      diagnostics.push({
        from,
        to,
        severity: "error",
        message: diag.message || "Parse error",
        source: diag.code || "rego_parse_error",
        actions: []
      });
      continue;
    }
    // Regal format (default)
    const { location, description, level, category, title, related_resources } = diag;
    if (!location || !location.row) continue;
    const line = doc.line(location.row); // 1-based
    const from = line.from + ((location.col || 1) - 1);
    const to =
      location.end && location.end.row === location.row
        ? line.from + ((location.end.col || line.length + 1) - 1)
        : line.to;
    diagnostics.push({
      from,
      to,
      severity: level === "error" ? "error" : "warning",
      message: description,
      source: `${category}/${title}`,
      actions: related_resources
        ? related_resources.map(res => ({
            name: res.description || res.ref,
            apply(view, from, to) {
              window.open(res.ref, "_blank");
            }
          }))
        : []
    });
  }
  return diagnostics;
}

function debounce(fn, wait) {
  let timeout;
  return function(...args) {
    clearTimeout(timeout);
    timeout = setTimeout(() => fn.apply(this, args), wait);
  };
}

function shouldIgnoreLine(text) {
  // Matches: starts with whitespace then #, or blank line, or starts with whitespace then package
  return (
    /^\s*#/.test(text) ||
    /^\s*$/.test(text) ||
    /^\s*package\b/.test(text)
  );
}
