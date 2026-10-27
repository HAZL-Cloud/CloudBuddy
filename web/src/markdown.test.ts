import { describe, expect, it } from 'vitest'
import { parseInline, parseMarkdown, safeHref, type Block, type Inline } from './markdown'

// A compact HTML-like picture of the tree, so the expectations read like the output.
function inl(nodes: Inline[]): string {
  return nodes
    .map((n) => {
      switch (n.t) {
        case 'text':
          return n.text
        case 'br':
          return '<br>'
        case 'code':
          return `<code>${n.text}</code>`
        case 'link':
          return `<a ${n.href}>${inl(n.children)}</a>`
        default:
          return `<${n.t}>${inl(n.children)}</${n.t}>`
      }
    })
    .join('')
}

function html(blocks: Block[]): string {
  return blocks
    .map((b) => {
      switch (b.t) {
        case 'p':
          return `<p>${inl(b.children)}</p>`
        case 'heading':
          return `<h${b.level}>${inl(b.children)}</h${b.level}>`
        case 'code':
          return `<pre${b.lang ? ` ${b.lang}` : ''}>${b.text}</pre>`
        case 'list': {
          const tag = b.ordered ? 'ol' : 'ul'
          const start = b.ordered && b.start !== 1 ? ` ${b.start}` : ''
          return `<${tag}${start}>${b.items.map((item) => `<li>${html(item)}</li>`).join('')}</${tag}>`
        }
        case 'quote':
          return `<quote>${html(b.children)}</quote>`
        case 'table': {
          const row = (cells: Inline[][]) => cells.map((c, k) => `<td${b.align[k] ? ` ${b.align[k]}` : ''}>${inl(c)}</td>`).join('')
          return `<table><tr>${row(b.head)}</tr>${b.rows.map((r) => `<tr>${row(r)}</tr>`).join('')}</table>`
        }
        case 'hr':
          return '<hr>'
      }
    })
    .join('')
}

const md = (text: string) => html(parseMarkdown(text))
const line = (text: string) => inl(parseInline(text))

// Shaped like a real agent reply (IDs and addresses are documentation examples).
const AGENT_REPLY = `I haven't deployed memos. The sandbox blocked the SSH connection ("Operation not permitted").

**What's done:**
- **Port 8501 needs no change.** The security group already allows 8501–8509.
- **The VM is ready.** It runs Amazon Linux 2023.

**To finish, either:**
1. **Allow SSH in the sandbox.** Add \`203.0.113.10:22\` to the sandbox network settings.
2. **Run it yourself.** Connect with:
   \`\`\`bash
   aws ec2-instance-connect ssh --instance-id i-0123456789abcdef0 --region us-east-1
   \`\`\`
   Then on the VM:
   \`\`\`
   sudo docker run -d --name memos \\
     -p 8501:5230 neosmemostech/memos:stable
   \`\`\`

Memos would then be at http://203.0.113.10:8501. Its data is kept in \`/opt/memos\`.

**Before you deploy:**
- **This is a production server.** It is tagged \`is_latest_production = yes\`.
- Port 8501 is open to everyone, so is_latest_production matters.`

describe('blocks', () => {
  it('reads a real agent reply as paragraphs, lists and code', () => {
    expect(md(AGENT_REPLY)).toBe(
      [
        '<p>I haven\'t deployed memos. The sandbox blocked the SSH connection ("Operation not permitted").</p>',
        "<p><strong>What's done:</strong></p>",
        '<ul><li><p><strong>Port 8501 needs no change.</strong> The security group already allows 8501–8509.</p></li>',
        '<li><p><strong>The VM is ready.</strong> It runs Amazon Linux 2023.</p></li></ul>',
        '<p><strong>To finish, either:</strong></p>',
        '<ol><li><p><strong>Allow SSH in the sandbox.</strong> Add <code>203.0.113.10:22</code> to the sandbox network settings.</p></li>',
        '<li><p><strong>Run it yourself.</strong> Connect with:</p>',
        '<pre bash>aws ec2-instance-connect ssh --instance-id i-0123456789abcdef0 --region us-east-1</pre>',
        '<p>Then on the VM:</p>',
        '<pre>sudo docker run -d --name memos \\\n  -p 8501:5230 neosmemostech/memos:stable</pre></li></ol>',
        '<p>Memos would then be at <a http://203.0.113.10:8501>http://203.0.113.10:8501</a>. Its data is kept in <code>/opt/memos</code>.</p>',
        '<p><strong>Before you deploy:</strong></p>',
        '<ul><li><p><strong>This is a production server.</strong> It is tagged <code>is_latest_production = yes</code>.</p></li>',
        '<li><p>Port 8501 is open to everyone, so is_latest_production matters.</p></li></ul>',
      ].join(''),
    )
  })

  it('keeps single line breaks inside a paragraph', () => {
    expect(md('one\ntwo\n\nthree')).toBe('<p>one<br>two</p><p>three</p>')
    expect(md('a\r\nb')).toBe('<p>a<br>b</p>')
  })

  it('reads headings, and leaves #tags alone', () => {
    expect(md('# Title\n## Sub ##\n###### Six\n#tag\n####### seven')).toBe('<h1>Title</h1><h2>Sub</h2><h6>Six</h6><p>#tag<br>####### seven</p>')
    expect(md('Intro\n## Next')).toBe('<p>Intro</p><h2>Next</h2>')
  })

  it('reads fenced code with ``` or ~~~, and runs an unclosed fence to the end', () => {
    expect(md('```go\nfunc a() {}\n\n  x := 1\n```\nafter')).toBe('<pre go>func a() {}\n\n  x := 1</pre><p>after</p>')
    expect(md('~~~\n**not bold**\n~~~')).toBe('<pre>**not bold**</pre>')
    expect(md('````\n```\ninner\n```\n````')).toBe('<pre>```\ninner\n```</pre>')
    expect(md('text\n```\nopen')).toBe('<p>text</p><pre>open</pre>')
  })

  it('handles code fences under a numbered item that are not indented', () => {
    expect(md('1. Connect with:\n```\nssh vm\n```\nThen run it.\n2. Done')).toBe(
      '<ol><li><p>Connect with:</p></li></ol><pre>ssh vm</pre><p>Then run it.</p><ol 2><li><p>Done</p></li></ol>',
    )
    // Inside an item's fence, under-indented lines still belong to the item.
    expect(md('1. Run:\n   ```\nsudo dnf install docker\n   ```\n2. Next')).toBe(
      '<ol><li><p>Run:</p><pre>sudo dnf install docker</pre></li><li><p>Next</p></li></ol>',
    )
  })

  it('nests lists by indentation and keeps the start number', () => {
    expect(md('- a\n  - b\n    - c\n- d')).toBe('<ul><li><p>a</p><ul><li><p>b</p><ul><li><p>c</p></li></ul></li></ul></li><li><p>d</p></li></ul>')
    expect(md('3. three\n4. four')).toBe('<ol 3><li><p>three</p></li><li><p>four</p></li></ol>')
    expect(md('1. step\n  - detail\n2. step')).toBe('<ol><li><p>step</p><ul><li><p>detail</p></li></ul></li><li><p>step</p></li></ol>')
    expect(md('* star\n+ plus')).toBe('<ul><li><p>star</p></li><li><p>plus</p></li></ul>')
  })

  it('keeps a list with blank lines between items as one list', () => {
    expect(md('1. a\n\n2. b\n\n   more b\n\nafter')).toBe('<ol><li><p>a</p></li><li><p>b</p><p>more b</p></li></ol><p>after</p>')
  })

  it('lets a list interrupt a paragraph', () => {
    expect(md('Either:\n1. this\n2. that')).toBe('<p>Either:</p><ol><li><p>this</p></li><li><p>that</p></li></ol>')
    expect(md('Either:\n- this')).toBe('<p>Either:</p><ul><li><p>this</p></li></ul>')
    expect(md('Then:\n3. third')).toBe('<p>Then:</p><ol 3><li><p>third</p></li></ol>')
  })

  it('does not read emphasis or numbers as list markers', () => {
    expect(md('**bold** start\n*it* too\n1.5 million\n-dash')).toBe('<p><strong>bold</strong> start<br><em>it</em> too<br>1.5 million<br>-dash</p>')
  })

  it('reads quotes, nested quotes and rules', () => {
    expect(md('> quoted\n> > deeper\n\n---\n***\n- - -')).toBe('<quote><p>quoted</p><quote><p>deeper</p></quote></quote><hr><hr><hr>')
  })

  it('reads tables with alignment, escaped pipes and ragged rows', () => {
    expect(md('| Name | Size | Note |\n|:--|--:|:-:|\n| a | 1 | x \\| y |\n| b |\n\nafter')).toBe(
      '<table><tr><td left>Name</td><td right>Size</td><td center>Note</td></tr><tr><td left>a</td><td right>1</td><td center>x | y</td></tr><tr><td left>b</td><td right></td><td center></td></tr></table><p>after</p>',
    )
    expect(md('A | B\n--- | ---\n`x` | **y**')).toBe('<table><tr><td>A</td><td>B</td></tr><tr><td><code>x</code></td><td><strong>y</strong></td></tr></table>')
    expect(md('Intro\n| a | b |\n|---|---|\n| 1 | 2 |')).toBe('<p>Intro</p><table><tr><td>a</td><td>b</td></tr><tr><td>1</td><td>2</td></tr></table>')
  })

  it('leaves pipes without a delimiter row as text', () => {
    expect(md('a | b\nc | d')).toBe('<p>a | b<br>c | d</p>')
    expect(md('| a | b |\n|---|')).toBe('<p>| a | b |<br>|---|</p>')
  })
})

describe('inline', () => {
  it('reads code, bold, italic and strike-through', () => {
    expect(line('`a` **b** __c__ *d* _e_ ~~f~~')).toBe('<code>a</code> <strong>b</strong> <strong>c</strong> <em>d</em> <em>e</em> <del>f</del>')
    expect(line('***both*** and **bold with *italic* inside**')).toBe('<em><strong>both</strong></em> and <strong>bold with <em>italic</em> inside</strong>')
    expect(line('**see `x`**')).toBe('<strong>see <code>x</code></strong>')
  })

  it('never italicises snake_case or spaced asterisks', () => {
    expect(line('is_latest_production and snake_case_name')).toBe('is_latest_production and snake_case_name')
    expect(line('2 * 3 * 4 and a*b')).toBe('2 * 3 * 4 and a*b')
    expect(line('~5 min and ~~~ three')).toBe('~5 min and ~~~ three')
  })

  it('leaves unclosed markers as text', () => {
    expect(line('**bold')).toBe('**bold')
    expect(line('`code')).toBe('`code')
    expect(line('*a **b')).toBe('*a **b')
    expect(line('[text](')).toBe('[text](')
    expect(line('[text]')).toBe('[text]')
    expect(line('~~strike')).toBe('~~strike')
    expect(line('**a*')).toBe('*<em>a</em>')
  })

  it('reads code spans with longer fences and keeps their insides literal', () => {
    expect(line('``a `b` c``')).toBe('<code>a `b` c</code>')
    expect(line('` **x** `')).toBe('<code>**x**</code>')
    expect(line('\\`not code`')).toBe('`not code`')
  })

  it('reads backslash escapes', () => {
    expect(line('\\*not italic\\* and C:\\Users')).toBe('*not italic* and C:\\Users')
  })

  it('reads links with titles, brackets in the address and formatting in the text', () => {
    expect(line('[docs](https://example.com/a "Title")')).toBe('<a https://example.com/a>docs</a>')
    expect(line('[Go](https://en.wikipedia.org/wiki/Go_(programming_language)) next')).toBe(
      '<a https://en.wikipedia.org/wiki/Go_(programming_language)>Go</a> next',
    )
    expect(line('[**bold** `code`](https://x.dev)')).toBe('<a https://x.dev><strong>bold</strong> <code>code</code></a>')
    expect(line('[a [b](https://x.dev)')).toBe('[a <a https://x.dev>b</a>')
    expect(line('[link](<https://x.dev/a b>)')).toBe('[link](<<a https://x.dev/a>https://x.dev/a</a> b>)')
  })

  it('finds bare web addresses and trims the punctuation after them', () => {
    expect(line('See https://example.com/path.')).toBe('See <a https://example.com/path>https://example.com/path</a>.')
    expect(line('(see https://example.com/x)')).toBe('(see <a https://example.com/x>https://example.com/x</a>)')
    expect(line('**https://example.com**')).toBe('<strong><a https://example.com>https://example.com</a></strong>')
    expect(line('<https://example.com/a_b_c>')).toBe('<a https://example.com/a_b_c>https://example.com/a_b_c</a>')
    expect(line('xhttps://no.link and https:// alone')).toBe('xhttps://no.link and https:// alone')
  })
})

describe('safeHref', () => {
  it('allows only http and https', () => {
    expect(safeHref('https://example.com/a?b=1')).toBe('https://example.com/a?b=1')
    expect(safeHref('http://127.0.0.1:8501')).toBe('http://127.0.0.1:8501/')
    for (const bad of ['javascript:alert(1)', 'JAVASCRIPT:alert(1)', ' javascript:x', 'data:text/html,hi', 'mailto:a@b.c', 'file:///etc/passwd', '/api/x', 'internal/server.go', 'vbscript:x']) {
      expect(safeHref(bad), bad).toBeNull()
    }
  })
})

describe('limits', () => {
  it('caps nesting depth', () => {
    const quotes = md('>'.repeat(5000) + ' deep')
    expect(quotes.match(/<quote>/g)?.length).toBe(9)
    const lists = md(Array.from({ length: 40 }, (_, i) => `${'  '.repeat(i)}- level ${i}`).join('\n'))
    expect(lists.match(/<ul>/g)?.length).toBe(9)
    const inline = line('*a _'.repeat(40) + 'x' + '_ a*'.repeat(40))
    expect(inline.match(/<em>/g)?.length).toBe(12)
    expect(inline).toContain('x')
  })

  it('stays fast on 64 KiB of hostile text', () => {
    const size = 64 * 1024
    const fill = (unit: string) => unit.repeat(Math.ceil(size / unit.length)).slice(0, size)
    const ticks = Array.from({ length: 4000 }, (_, i) => '`'.repeat((i % 40) + 1) + 'x').join('').slice(0, size)
    const cases = [
      fill('*'),
      fill('_a'),
      fill('*a _b '),
      fill('**a __b ~~c '),
      fill('['),
      fill('[a]('),
      fill('[a]('.repeat(100) + ')'),
      fill('](') ,
      fill('<https://'),
      fill('https://a'),
      fill('http://(('),
      fill('>'),
      fill('> '),
      fill('- '),
      fill('1. '),
      fill('  - x\n'),
      fill('| a '),
      fill('| a |\n|---|\n'),
      fill('```\n'),
      fill('`a'),
      ticks,
      fill('\\*'),
      fill('#'),
      fill('\n\n'),
      fill(' \n- a\n\n'),
    ]
    for (const text of cases) {
      const start = performance.now()
      parseMarkdown(text)
      const took = performance.now() - start
      expect(took, JSON.stringify(text.slice(0, 24))).toBeLessThan(1000)
    }
  })
})
