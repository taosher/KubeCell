export type TokenType =
  | "plain"
  | "comment"
  | "key"
  | "string"
  | "number"
  | "flag"
  | "command"
  | "keyword"
  | "punct";

export type Token = {
  text: string;
  type: TokenType;
};

export type Language = "yaml" | "bash" | "shell" | "go" | "json" | "text";

const GO_KEYWORDS =
  /\b(package|import|func|return|if|else|for|range|var|const|type|struct|interface|map|chan|go|defer|select|case|switch|break|continue|nil|true|false)\b/;

function findCommentStart(line: string): number {
  let inSingle = false;
  let inDouble = false;
  for (let i = 0; i < line.length; i += 1) {
    const char = line[i];
    if (char === "'" && !inDouble) inSingle = !inSingle;
    else if (char === '"' && !inSingle) inDouble = !inDouble;
    else if (char === "#" && !inSingle && !inDouble) {
      if (i === 0 || /\s/.test(line[i - 1])) return i;
    }
  }
  return -1;
}

function findGoCommentStart(line: string): number {
  let inDouble = false;
  let inBacktick = false;
  for (let i = 0; i < line.length; i += 1) {
    const char = line[i];
    if (char === '"' && !inBacktick) inDouble = !inDouble;
    else if (char === "`") inBacktick = !inBacktick;
    else if (char === "/" && line[i + 1] === "/" && !inDouble && !inBacktick) return i;
  }
  return -1;
}

function tokenizeYaml(code: string): Token[] {
  const tokens: Token[] = [];
  const keyMatch = code.match(/^(\s*)([A-Za-z0-9_.\-/]+)(:)(\s*)/);
  let rest = code;
  if (keyMatch) {
    const [, indent, key, colon, space] = keyMatch;
    if (indent) tokens.push({ text: indent, type: "plain" });
    tokens.push({ text: key, type: "key" });
    tokens.push({ text: colon, type: "punct" });
    if (space) tokens.push({ text: space, type: "plain" });
    rest = code.slice(keyMatch[0].length);
  } else {
    const listMatch = code.match(/^(\s*)(-)(\s+)/);
    if (listMatch) {
      const [, indent, dash, space] = listMatch;
      if (indent) tokens.push({ text: indent, type: "plain" });
      tokens.push({ text: dash, type: "punct" });
      tokens.push({ text: space, type: "plain" });
      rest = code.slice(listMatch[0].length);
    }
  }

  const pattern =
    /("[^"]*"|'[^']*'|\{[^}]*\}|\[[^\]]*\]|\btrue\b|\bfalse\b|\bnull\b|\b\d+(?:\.\d+)?(?:[A-Za-z]+)?\b)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(rest)) !== null) {
    if (match.index > lastIndex) {
      tokens.push({ text: rest.slice(lastIndex, match.index), type: "plain" });
    }
    const value = match[0];
    tokens.push({
      text: value,
      type: /^["'{[ ]/.test(value) || value.startsWith("{") || value.startsWith("[")
        ? "string"
        : /^(true|false|null)$/.test(value)
          ? "keyword"
          : "number",
    });
    lastIndex = match.index + value.length;
  }
  if (lastIndex < rest.length) tokens.push({ text: rest.slice(lastIndex), type: "plain" });
  return tokens;
}

function tokenizeBash(code: string): Token[] {
  const tokens: Token[] = [];
  const pattern =
    /("[^"]*"|'[^']*'|\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*|--?[A-Za-z][A-Za-z0-9-]*|\|\||&&|\||>|<|\b\d+\b)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let first = true;
  const pushPlain = (text: string) => {
    if (!text) return;
    if (first && text.trim().length > 0) {
      const leading = text.match(/^\s*/)?.[0] ?? "";
      const word = text.trimEnd();
      if (leading) tokens.push({ text: leading, type: "plain" });
      tokens.push({ text: word, type: "command" });
      first = false;
      return;
    }
    tokens.push({ text, type: "plain" });
  };
  while ((match = pattern.exec(code)) !== null) {
    if (match.index > lastIndex) pushPlain(code.slice(lastIndex, match.index));
    const value = match[0];
    let type: TokenType = "plain";
    if (/^["']/.test(value)) type = "string";
    else if (value.startsWith("$")) type = "string";
    else if (value.startsWith("-")) type = "flag";
    else if (/^(\|\||&&|\||>|<)$/.test(value)) type = "punct";
    else if (/^\d+$/.test(value)) type = "number";
    tokens.push({ text: value, type });
    lastIndex = match.index + value.length;
    first = false;
  }
  if (lastIndex < code.length) pushPlain(code.slice(lastIndex));
  return tokens;
}

function tokenizeGo(code: string): Token[] {
  const tokens: Token[] = [];
  const pattern = /("[^"]*"|`[^`]*`|'[^']*')/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  const pushCode = (text: string) => {
    if (!text) return;
    let index = 0;
    const wordPattern = new RegExp(GO_KEYWORDS.source, "g");
    let wordMatch: RegExpExecArray | null;
    while ((wordMatch = wordPattern.exec(text)) !== null) {
      if (wordMatch.index > index) tokens.push({ text: text.slice(index, wordMatch.index), type: "plain" });
      tokens.push({ text: wordMatch[0], type: "keyword" });
      index = wordMatch.index + wordMatch[0].length;
    }
    if (index < text.length) tokens.push({ text: text.slice(index), type: "plain" });
  };
  while ((match = pattern.exec(code)) !== null) {
    if (match.index > lastIndex) pushCode(code.slice(lastIndex, match.index));
    tokens.push({ text: match[0], type: "string" });
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < code.length) pushCode(code.slice(lastIndex));
  return tokens;
}

function tokenizeJson(code: string): Token[] {
  const tokens: Token[] = [];
  const pattern =
    /("(?:[^"\\]|\\.)*"(?=\s*:)|"(?:[^"\\]|\\.)*"|\btrue\b|\bfalse\b|\bnull\b|-?\b\d+(?:\.\d+)?\b|[{}[\],:])/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(code)) !== null) {
    if (match.index > lastIndex) tokens.push({ text: code.slice(lastIndex, match.index), type: "plain" });
    const value = match[0];
    let type: TokenType = "plain";
    if (value.startsWith('"')) {
      type = /"(?:[^"\\]|\\.)*"(?=\s*:)/.test(value) ? "key" : "string";
    } else if (/^(true|false|null)$/.test(value)) type = "keyword";
    else if (/^-?\d/.test(value)) type = "number";
    else type = "punct";
    tokens.push({ text: value, type });
    lastIndex = match.index + value.length;
  }
  if (lastIndex < code.length) tokens.push({ text: code.slice(lastIndex), type: "plain" });
  return tokens;
}

export function highlightLine(line: string, language: Language): Token[] {
  if (language === "text") return [{ text: line, type: "plain" }];

  if (language === "go") {
    const comment = findGoCommentStart(line);
    if (comment >= 0) {
      const code = line.slice(0, comment);
      const tokens = tokenizeGo(code);
      tokens.push({ text: line.slice(comment), type: "comment" });
      return tokens;
    }
    return tokenizeGo(line);
  }

  const comment = findCommentStart(line);
  const code = comment >= 0 ? line.slice(0, comment) : line;
  const tokens =
    language === "yaml" ? tokenizeYaml(code) : language === "json" ? tokenizeJson(code) : tokenizeBash(code);
  if (comment >= 0) tokens.push({ text: line.slice(comment), type: "comment" });
  return tokens;
}

const TOKEN_CLASS: Record<TokenType, string> = {
  plain: "",
  comment: "text-slate-500 italic",
  key: "text-sky-300",
  string: "text-emerald-300/90",
  number: "text-amber-300/90",
  flag: "text-violet-300/90",
  command: "text-sky-200 font-medium",
  keyword: "text-fuchsia-300/90",
  punct: "text-slate-400",
};

export function tokenClassName(type: TokenType) {
  return TOKEN_CLASS[type];
}
