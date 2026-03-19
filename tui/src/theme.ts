import { SyntaxStyle } from "@opentui/core"

// OpenCode dark theme adapted with orange accent
export const theme = {
  // Backgrounds
  background: "#0a0a0a",
  backgroundPanel: "#141414",
  backgroundElement: "#1e1e1e",
  backgroundMenu: "#282828",

  // Text
  text: "#eeeeee",
  textMuted: "#808080",

  // Primary (orange like OpenCode)
  primary: "#fab283",
  secondary: "#5c9cf5",
  accent: "#9d7cd8",

  // Semantic
  error: "#e06c75",
  warning: "#f5a742",
  success: "#7fd88f",
  info: "#56b6c2",

  // Borders
  border: "#484848",
  borderActive: "#606060",
  borderSubtle: "#3c3c3c",

  // Diff
  diffAdded: "#4fd6be",
  diffRemoved: "#c53b53",
  diffAddedBg: "#20303b",
  diffRemovedBg: "#37222c",
  diffContextBg: "#141414",
  diffHighlightAdded: "#b8db87",
  diffHighlightRemoved: "#e26a75",
  diffLineNumber: "#1e1e1e",
  diffAddedLineNumberBg: "#1b2b34",
  diffRemovedLineNumberBg: "#2d1f26",

  // Syntax
  syntaxComment: "#808080",
  syntaxKeyword: "#9d7cd8",
  syntaxFunction: "#fab283",
  syntaxVariable: "#e06c75",
  syntaxString: "#7fd88f",
  syntaxNumber: "#f5a742",
  syntaxType: "#e5c07b",
  syntaxOperator: "#56b6c2",
  syntaxPunctuation: "#eeeeee",
}

export function createSyntax(): SyntaxStyle {
  return SyntaxStyle.fromTheme([
    { scope: ["comment"], style: { foreground: theme.syntaxComment } },
    { scope: ["keyword"], style: { foreground: theme.syntaxKeyword } },
    { scope: ["entity.name.function"], style: { foreground: theme.syntaxFunction } },
    { scope: ["variable"], style: { foreground: theme.syntaxVariable } },
    { scope: ["string"], style: { foreground: theme.syntaxString } },
    { scope: ["constant.numeric"], style: { foreground: theme.syntaxNumber } },
    { scope: ["entity.name.type"], style: { foreground: theme.syntaxType } },
    { scope: ["keyword.operator"], style: { foreground: theme.syntaxOperator } },
    { scope: ["punctuation"], style: { foreground: theme.syntaxPunctuation } },
    { scope: ["markup.heading"], style: { foreground: theme.accent } },
    { scope: ["markup.bold"], style: { foreground: theme.warning, bold: true } },
    { scope: ["markup.italic"], style: { foreground: theme.warning, italic: true } },
    { scope: ["markup.link"], style: { foreground: theme.primary } },
    { scope: ["markup.code"], style: { foreground: theme.success } },
    { scope: ["markup.list"], style: { foreground: theme.primary } },
    { scope: ["markup.quote"], style: { foreground: theme.warning } },
    { scope: ["diff.added"], style: { foreground: theme.diffAdded } },
    { scope: ["diff.deleted"], style: { foreground: theme.diffRemoved } },
  ])
}

export function subtleSyntax(): SyntaxStyle {
  return SyntaxStyle.fromTheme([
    { scope: ["comment"], style: { foreground: theme.textMuted } },
    { scope: ["keyword"], style: { foreground: theme.textMuted } },
    { scope: ["string"], style: { foreground: theme.textMuted } },
    { scope: ["variable"], style: { foreground: theme.textMuted } },
    { scope: ["entity.name.function"], style: { foreground: theme.textMuted } },
    { scope: ["constant.numeric"], style: { foreground: theme.textMuted } },
  ])
}
