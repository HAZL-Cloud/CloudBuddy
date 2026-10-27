/**
 * A small markdown reader for agent messages. It returns a plain tree that the Markdown component turns into
 * React elements, so nothing is ever read as HTML. Agent text can be up to 64 KiB of anything, so every pass is
 * linear in the text and nesting stops at a fixed depth.
 */

export type Inline =
  | { t: 'text'; text: string }
  | { t: 'code'; text: string }
  | { t: 'br' }
  | { t: 'strong' | 'em' | 'del'; children: Inline[] }
  | { t: 'link'; href: string; children: Inline[] }

export type Align = 'left' | 'center' | 'right' | null

export type Block =
  | { t: 'p'; children: Inline[] }
  | { t: 'heading'; level: number; children: Inline[] }
  | { t: 'code'; lang: string; text: string }
  | { t: 'list'; ordered: boolean; start: number; items: Block[][] }
  | { t: 'quote'; children: Block[] }
  | { t: 'table'; align: Align[]; head: Inline[][]; rows: Inline[][][] }
  | { t: 'hr' }

/** Lists and quotes deeper than this are shown as plain paragraphs. */
const MAX_DEPTH = 8
/** Bold, italic and strike-through deeper than this are shown as their literal characters. */
const MAX_INLINE_DEPTH = 12
/** Longer link targets are not links. */
const MAX_DEST = 2048

export function parseMarkdown(text: string): Block[] {
  return parseBlocks(text.replace(/\r\n?/g, '\n').split('\n').map(expandTabs), 0)
}

/** The link target if it is a plain web address, otherwise null: links to anything else are shown as text. */
export function safeHref(href: string): string | null {
  try {
    const url = new URL(href)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : null
  } catch {
    return null
  }
}

// ---------- blocks ----------

function expandTabs(line: string): string {
  let i = 0
  let lead = ''
  while (i < line.length && (line[i] === ' ' || line[i] === '\t')) lead += line[i++] === '\t' ? '    ' : ' '
  return i === 0 ? line : lead + line.slice(i)
}

const indentOf = (line: string) => {
  let n = 0
  while (n < line.length && line[n] === ' ') n++
  return n
}
const isBlank = (line: string) => indentOf(line) === line.length
const stripIndent = (line: string, n: number) => line.slice(Math.min(n, indentOf(line)))
const QUOTE = /^ {0,3}>/

interface Fence {
  indent: number
  ch: string
  len: number
  lang: string
}

function openFence(line: string): Fence | null {
  const indent = indentOf(line)
  const ch = line[indent]
  if (indent > 3 || (ch !== '`' && ch !== '~')) return null
  let end = indent
  while (line[end] === ch) end++
  if (end - indent < 3) return null
  const info = line.slice(end).trim()
  if (ch === '`' && info.includes('`')) return null
  return { indent, ch, len: end - indent, lang: info.split(/\s/)[0] ?? '' }
}

function closesFence(line: string, f: Fence): boolean {
  const indent = indentOf(line)
  if (indent > 3) return false
  let end = indent
  while (line[end] === f.ch) end++
  return end - indent >= f.len && isBlank(line.slice(end))
}

function heading(line: string): { level: number; text: string } | null {
  const indent = indentOf(line)
  if (indent > 3) return null
  let i = indent
  while (line[i] === '#') i++
  const level = i - indent
  if (level < 1 || level > 6 || (i < line.length && line[i] !== ' ')) return null
  let text = line.slice(i).trim()
  // An optional closing run of #s, when it stands apart from the text.
  let j = text.length
  while (j > 0 && text[j - 1] === '#') j--
  if (j === 0) text = ''
  else if (j < text.length && text[j - 1] === ' ') text = text.slice(0, j).trimEnd()
  return { level, text }
}

function isRule(line: string): boolean {
  if (indentOf(line) > 3) return false
  const s = line.trim()
  const ch = s[0]
  if (ch !== '-' && ch !== '*' && ch !== '_') return false
  let n = 0
  for (const c of s) {
    if (c === ch) n++
    else if (c !== ' ') return false
  }
  return n >= 3
}

interface Marker {
  indent: number
  ordered: boolean
  start: number
  /** Where the item's text starts; later lines indented this far belong to the item. */
  content: number
  empty: boolean
}

function listMarker(line: string): Marker | null {
  const indent = indentOf(line)
  let i = indent
  let ordered = false
  let start = 1
  const c = line[i]
  if (c === '-' || c === '*' || c === '+') i++
  else {
    let j = i
    while (j - i < 9 && line[j] !== undefined && line[j]! >= '0' && line[j]! <= '9') j++
    if (j === i || (line[j] !== '.' && line[j] !== ')')) return null
    ordered = true
    start = Number(line.slice(i, j))
    i = j + 1
  }
  if (i < line.length && line[i] !== ' ') return null
  let spaces = 0
  while (line[i + spaces] === ' ') spaces++
  const empty = i + spaces >= line.length
  return { indent, ordered, start, content: empty || spaces > 4 ? i + 1 : i + spaces, empty }
}

function splitRow(line: string): string[] {
  let s = line.trim()
  if (s.startsWith('|')) s = s.slice(1)
  if (s.endsWith('|') && !s.endsWith('\\|')) s = s.slice(0, -1)
  const cells: string[] = []
  let from = 0
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\' && s[i + 1] === '|') i++
    else if (s[i] === '|') {
      cells.push(s.slice(from, i))
      from = i + 1
    }
  }
  cells.push(s.slice(from))
  return cells.map((c) => c.replaceAll('\\|', '|').trim())
}

function delimiterRow(line: string): Align[] | null {
  if (!line.includes('|') || !line.includes('-')) return null
  const out: Align[] = []
  for (const cell of splitRow(line)) {
    if (!/^:?-+:?$/.test(cell)) return null
    const left = cell.startsWith(':')
    const right = cell.endsWith(':')
    out.push(left && right ? 'center' : right ? 'right' : left ? 'left' : null)
  }
  return out
}

function tableStart(lines: string[], i: number): { align: Align[]; head: string[] } | null {
  const header = lines[i]
  const next = lines[i + 1]
  if (header === undefined || next === undefined || !header.includes('|')) return null
  const align = delimiterRow(next)
  const head = align && splitRow(header)
  return align && head && head.length === align.length ? { align, head } : null
}

/** True when the line starts a block that ends the paragraph before it. */
function startsBlock(lines: string[], i: number): boolean {
  const line = lines[i] ?? ''
  if (openFence(line) || heading(line) || isRule(line) || QUOTE.test(line)) return true
  // Any numbered item may interrupt: agents number on after a code block, as in "2." after an unindented fence.
  const m = listMarker(line)
  if (m && !m.empty) return true
  return tableStart(lines, i) !== null
}

function parseBlocks(lines: string[], depth: number): Block[] {
  if (depth > MAX_DEPTH) {
    const text = lines.map((l) => l.trim()).join('\n').trim()
    return text ? [{ t: 'p', children: parseInline(text) }] : []
  }
  const out: Block[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]!
    if (isBlank(line)) {
      i++
      continue
    }

    const fence = openFence(line)
    if (fence) {
      const body: string[] = []
      i++
      // A fence that is never closed runs to the end of the message.
      while (i < lines.length && !closesFence(lines[i]!, fence)) body.push(stripIndent(lines[i++]!, fence.indent))
      i++
      out.push({ t: 'code', lang: fence.lang, text: body.join('\n') })
      continue
    }

    const h = heading(line)
    if (h) {
      out.push({ t: 'heading', level: h.level, children: parseInline(h.text) })
      i++
      continue
    }

    if (isRule(line)) {
      out.push({ t: 'hr' })
      i++
      continue
    }

    if (QUOTE.test(line)) {
      const inner: string[] = []
      while (i < lines.length && QUOTE.test(lines[i]!)) inner.push(lines[i++]!.replace(/^ {0,3}> ?/, ''))
      out.push({ t: 'quote', children: parseBlocks(inner, depth + 1) })
      continue
    }

    const table = tableStart(lines, i)
    if (table) {
      const width = table.align.length
      const cells = (row: string[]) => Array.from({ length: width }, (_, k) => parseInline(row[k] ?? ''))
      const rows: Inline[][][] = []
      i += 2
      while (i < lines.length && !isBlank(lines[i]!) && lines[i]!.includes('|') && !openFence(lines[i]!)) rows.push(cells(splitRow(lines[i++]!)))
      out.push({ t: 'table', align: table.align, head: cells(table.head), rows })
      continue
    }

    const marker = listMarker(line)
    if (marker) {
      const [list, next] = parseList(lines, i, marker, depth)
      out.push(list)
      i = next
      continue
    }

    const para = [line.trim()]
    i++
    while (i < lines.length && !isBlank(lines[i]!) && !startsBlock(lines, i)) para.push(lines[i++]!.trim())
    out.push({ t: 'p', children: parseInline(para.join('\n')) })
  }
  return out
}

function parseList(lines: string[], i: number, first: Marker, depth: number): [Block, number] {
  const items: Block[][] = []
  let cur: string[] = []
  let contentIndent = first.content
  let itemIndent = first.indent
  // Inside a code fence every line belongs to the item, however it is indented: agents often under-indent them.
  let fence: Fence | null = null
  const push = (text: string) => {
    cur.push(text)
    if (fence) {
      if (closesFence(text, fence)) fence = null
    } else fence = openFence(text)
  }
  const startItem = (m: Marker, line: string) => {
    if (cur.length) items.push(parseBlocks(cur, depth + 1))
    cur = []
    fence = null
    contentIndent = m.content
    itemIndent = m.indent
    push(line.slice(m.content))
  }

  startItem(first, lines[i]!)
  i++
  while (i < lines.length) {
    const line = lines[i]!
    if (fence) {
      push(stripIndent(line, contentIndent))
      i++
      continue
    }
    if (isBlank(line)) {
      let j = i + 1
      while (j < lines.length && isBlank(lines[j]!)) j++
      const next = lines[j]
      if (next === undefined) break
      const m = listMarker(next)
      if (indentOf(next) >= contentIndent) {
        for (let k = i; k < j; k++) cur.push('')
      } else if (!m || m.ordered !== first.ordered || isRule(next)) break
      i = j
      continue
    }
    const indent = indentOf(line)
    if (indent >= contentIndent) {
      push(line.slice(contentIndent))
      i++
      continue
    }
    const m = listMarker(line)
    if (m && !isRule(line)) {
      if (m.ordered === first.ordered) startItem(m, line)
      else if (indent > itemIndent) push(line.slice(indent)) // a nested list of the other kind, under-indented
      else break
      i++
      continue
    }
    // A plain line right after the item's text carries on its paragraph.
    if (startsBlock(lines, i)) break
    push(line.trim())
    i++
  }
  items.push(parseBlocks(cur, depth + 1))
  return [{ t: 'list', ordered: first.ordered, start: first.start, items }, i]
}

// ---------- inline ----------

interface Pair {
  kind: 'strong' | 'em' | 'del'
  id: number
  size: number
}

/** A run of *, _ or ~~ that may open or close emphasis. */
interface Delim {
  t: 'delim'
  ch: '*' | '_' | '~'
  /** Characters not yet used by a pair. */
  rem: number
  open: boolean
  close: boolean
  /** Pairs this run opens and closes, innermost first. */
  opens: Pair[]
  closes: Pair[]
}

type Atom = Inline | Delim

const ESCAPABLE = '!"#$%&\'()*+,-./:;<=>?@[\\]^_`{|}~'
const WS = /\s/
const PUNCT = /[\p{P}\p{S}]/u
const ALNUM = /[A-Za-z0-9]/
const TRAIL = '.,:;!?\'"*_~'
const isWs = (c: string | undefined) => c === undefined || WS.test(c)
const isPunct = (c: string | undefined) => c !== undefined && PUNCT.test(c)

/** For each position, the index of the next ch at or after it (or the length). */
function nextIndex(s: string, ch: string): Int32Array {
  const out = new Int32Array(s.length + 1)
  let next = s.length
  out[s.length] = next
  for (let i = s.length - 1; i >= 0; i--) {
    if (s[i] === ch) next = i
    out[i] = next
  }
  return out
}

/** Finds the backtick run that closes a code span. Queries must come in increasing position. */
function tickIndex(s: string) {
  const byLen = new Map<number, number[]>()
  for (let i = s.indexOf('`'); i >= 0; i = s.indexOf('`', i)) {
    let j = i
    while (s[j] === '`') j++
    const list = byLen.get(j - i)
    if (list) list.push(i)
    else byLen.set(j - i, [i])
    i = j
  }
  const cursor = new Map<number, number>()
  return (pos: number, len: number): number => {
    const list = byLen.get(len)
    if (!list) return -1
    let k = cursor.get(len) ?? 0
    while (k < list.length && list[k]! <= pos) k++
    cursor.set(len, k)
    return list[k] ?? -1
  }
}

function codeText(raw: string): string {
  const s = raw.replace(/\n/g, ' ')
  return s.length > 2 && s.startsWith(' ') && s.endsWith(' ') && s.trim() !== '' ? s.slice(1, -1) : s
}

function linkDest(s: string, start: number, paren: Int32Array): { href: string; end: number } | null {
  let end = paren[start]!
  if (end >= s.length || end - start > MAX_DEST) return null
  let raw = s.slice(start, end)
  // One level of brackets inside the address, as in https://en.wikipedia.org/wiki/Go_(programming_language).
  if (raw.includes('(')) {
    const outer = paren[end + 1]!
    if (outer >= s.length || outer - start > MAX_DEST) return null
    end = outer
    raw = s.slice(start, end)
  }
  raw = raw.trim()
  if (raw.includes('\n')) return null
  const space = raw.search(/\s/)
  let href = space < 0 ? raw : raw.slice(0, space)
  const title = space < 0 ? '' : raw.slice(space).trim()
  if (title && !/^(".*"|'.*')$/.test(title)) return null
  if (href.startsWith('<') && href.endsWith('>')) href = href.slice(1, -1)
  return href ? { href, end: end + 1 } : null
}

/** Where a bare web address ends: not on trailing punctuation, or on a bracket it did not open. */
function urlEnd(s: string, i: number): number {
  let j = i
  while (j < s.length && !WS.test(s[j]!) && s[j] !== '<' && s[j] !== '`') j++
  let balance = 0
  let square = 0
  for (let k = i; k < j; k++) {
    if (s[k] === '(') balance++
    else if (s[k] === ')') balance--
    else if (s[k] === '[') square++
    else if (s[k] === ']') square--
  }
  for (;;) {
    const c = s[j - 1]!
    if (TRAIL.includes(c)) j--
    else if (c === ')' && balance < 0) {
      j--
      balance++
    } else if (c === ']' && square < 0) {
      j--
      square++
    } else return j
  }
}

const autolink = (url: string): Inline => ({ t: 'link', href: url, children: [{ t: 'text', text: url }] })

export function parseInline(s: string, links = true): Inline[] {
  const n = s.length
  const atoms: Atom[] = []
  let from = 0
  const flush = (to: number) => {
    if (to > from) atoms.push({ t: 'text', text: s.slice(from, to) })
  }
  const closer = tickIndex(s)
  const brackets = links && s.includes('[') ? { open: nextIndex(s, '['), close: nextIndex(s, ']'), paren: nextIndex(s, ')') } : null

  let i = 0
  while (i < n) {
    const c = s[i]!
    if (c === '\\' && i + 1 < n && ESCAPABLE.includes(s[i + 1]!)) {
      flush(i)
      atoms.push({ t: 'text', text: s[i + 1]! })
      i += 2
      from = i
    } else if (c === '`') {
      let len = 1
      while (s[i + len] === '`') len++
      const end = closer(i, len)
      if (end >= 0) {
        flush(i)
        atoms.push({ t: 'code', text: codeText(s.slice(i + len, end)) })
        i = end + len
        from = i
      } else i += len
    } else if (c === '[' && brackets) {
      const close = brackets.close[i + 1]!
      const dest = close < n && close > i + 1 && brackets.open[i + 1]! > close && s[close + 1] === '(' ? linkDest(s, close + 2, brackets.paren) : null
      if (dest) {
        flush(i)
        atoms.push({ t: 'link', href: dest.href, children: parseInline(s.slice(i + 1, close), false) })
        i = dest.end
        from = i
      } else i++
    } else if (c === '<' && links && (s.startsWith('https://', i + 1) || s.startsWith('http://', i + 1))) {
      let j = i + 1
      while (j < n && s[j] !== '>' && s[j] !== '<' && !WS.test(s[j]!)) j++
      if (s[j] === '>') {
        flush(i)
        atoms.push(autolink(s.slice(i + 1, j)))
        i = j + 1
        from = i
      } else i++
    } else if (c === 'h' && links && !(i > 0 && ALNUM.test(s[i - 1]!)) && (s.startsWith('https://', i) || s.startsWith('http://', i))) {
      const end = urlEnd(s, i)
      if (end > i + (s.startsWith('https://', i) ? 8 : 7)) {
        flush(i)
        atoms.push(autolink(s.slice(i, end)))
        i = end
        from = i
      } else i++
    } else if (c === '*' || c === '_' || c === '~') {
      let j = i + 1
      while (s[j] === c) j++
      if (c === '~' && j - i !== 2) {
        i = j
        continue
      }
      flush(i)
      const prev = s[i - 1]
      const next = s[j]
      const left = !isWs(next) && (!isPunct(next) || isWs(prev) || isPunct(prev))
      const right = !isWs(prev) && (!isPunct(prev) || isWs(next) || isPunct(next))
      // Underscores inside a word, as in snake_case, never start or end emphasis.
      const open = c === '_' ? left && (!right || isPunct(prev)) : left
      const close = c === '_' ? right && (!left || isPunct(next)) : right
      atoms.push({ t: 'delim', ch: c, rem: j - i, open, close, opens: [], closes: [] })
      i = j
      from = i
    } else if (c === '\n') {
      flush(i)
      atoms.push({ t: 'br' })
      i++
      from = i
    } else i++
  }
  flush(n)
  pairDelims(atoms)
  return build(atoms)
}

/**
 * Pairs openers and closers, nearest first. Openers left between a matched pair can no longer close anything,
 * so they leave the stack; each run enters and leaves it once, which keeps this linear.
 */
function pairDelims(atoms: Atom[]): void {
  const stack: Delim[] = []
  const open = { '*': 0, _: 0, '~': 0 }
  let id = 0
  for (const a of atoms) {
    if (a.t !== 'delim') continue
    while (a.close && a.rem > 0 && open[a.ch] > 0) {
      let k = stack.length - 1
      while (stack[k]!.ch !== a.ch) k--
      for (let x = stack.length - 1; x > k; x--) open[stack[x]!.ch]--
      stack.length = k + 1
      const o = stack[k]!
      const size = a.ch === '~' ? 2 : o.rem >= 2 && a.rem >= 2 ? 2 : 1
      const pair: Pair = { kind: a.ch === '~' ? 'del' : size === 2 ? 'strong' : 'em', id: id++, size }
      o.opens.push(pair)
      a.closes.push(pair)
      o.rem -= size
      a.rem -= size
      if (o.rem === 0) {
        stack.pop()
        open[o.ch]--
      }
    }
    if (a.open && a.rem > 0) {
      stack.push(a)
      open[a.ch]++
    }
  }
}

function build(atoms: Atom[]): Inline[] {
  const root: Inline[] = []
  const frames: { id: number; kind: Pair['kind']; children: Inline[] }[] = []
  const flat = new Set<number>()
  const top = () => frames[frames.length - 1]?.children ?? root
  const text = (t: string) => {
    if (!t) return
    const out = top()
    const last = out[out.length - 1]
    if (last?.t === 'text') last.text += t
    else out.push({ t: 'text', text: t })
  }
  for (const a of atoms) {
    if (a.t === 'text') text(a.text)
    else if (a.t !== 'delim') top().push(a)
    else {
      // Closers use the run's first characters, openers its last, and what is left over stays as text.
      for (const p of a.closes) {
        const f = frames[frames.length - 1]
        if (flat.has(p.id) || !f || f.id !== p.id) text(a.ch.repeat(p.size))
        else {
          frames.pop()
          top().push({ t: f.kind, children: f.children })
        }
      }
      text(a.ch.repeat(a.rem))
      for (let k = a.opens.length - 1; k >= 0; k--) {
        const p = a.opens[k]!
        if (frames.length >= MAX_INLINE_DEPTH) {
          flat.add(p.id)
          text(a.ch.repeat(p.size))
        } else frames.push({ id: p.id, kind: p.kind, children: [] })
      }
    }
  }
  // Every pair closes by construction; this only keeps the text if that is ever not so.
  for (let f = frames.pop(); f; f = frames.pop()) for (const child of f.children) top().push(child)
  return root
}
