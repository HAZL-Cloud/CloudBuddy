import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { parseMarkdown, safeHref, type Align, type Block, type Inline } from '../markdown'
import { useToast } from './Toast'

/** Agent text as formatted markdown. Every piece is a React element or plain text, never HTML. */
export function Markdown({ text }: { text: string }) {
  const blocks = useMemo(() => parseMarkdown(text), [text])
  return <div className="md">{renderBlocks(blocks)}</div>
}

// The chat title is the page's h1, so message headings start at h3.
const HEADINGS = ['h3', 'h4', 'h5', 'h6'] as const

const alignClass = (a: Align | undefined) => (a ? `al-${a}` : undefined)

function renderBlocks(blocks: Block[]): ReactNode[] {
  return blocks.map((b, i) => {
    switch (b.t) {
      case 'p':
        return <p key={i}>{renderInline(b.children)}</p>
      case 'heading': {
        const Tag = HEADINGS[Math.min(b.level, HEADINGS.length) - 1] ?? 'h6'
        return (
          <Tag key={i} className={`md-h md-h${b.level}`}>
            {renderInline(b.children)}
          </Tag>
        )
      }
      case 'code':
        return <CodeBlock key={i} lang={b.lang} text={b.text} />
      case 'list': {
        const items = b.items.map((item, k) => <li key={k}>{renderBlocks(item)}</li>)
        return b.ordered ? (
          <ol key={i} start={b.start === 1 ? undefined : b.start}>
            {items}
          </ol>
        ) : (
          <ul key={i}>{items}</ul>
        )
      }
      case 'quote':
        return <blockquote key={i}>{renderBlocks(b.children)}</blockquote>
      case 'table':
        return (
          <div key={i} className="md-table" role="region" aria-label="Table" tabIndex={0}>
            <table>
              <thead>
                <tr>
                  {b.head.map((cell, k) => (
                    <th key={k} className={alignClass(b.align[k])}>
                      {renderInline(cell)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {b.rows.map((row, r) => (
                  <tr key={r}>
                    {row.map((cell, k) => (
                      <td key={k} className={alignClass(b.align[k])}>
                        {renderInline(cell)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      case 'hr':
        return <hr key={i} />
    }
  })
}

function renderInline(nodes: Inline[]): ReactNode[] {
  return nodes.map((n, i) => {
    switch (n.t) {
      case 'text':
        return <Fragment key={i}>{n.text}</Fragment>
      case 'br':
        return <br key={i} />
      case 'code':
        return <code key={i}>{n.text}</code>
      case 'strong':
        return <strong key={i}>{renderInline(n.children)}</strong>
      case 'em':
        return <em key={i}>{renderInline(n.children)}</em>
      case 'del':
        return <del key={i}>{renderInline(n.children)}</del>
      case 'link': {
        // Only web addresses become links. Anything else (javascript:, data:, file paths) is shown as its text.
        const href = safeHref(n.href)
        return href ? (
          <a key={i} href={href} title={href} target="_blank" rel="noopener noreferrer">
            {renderInline(n.children)}
          </a>
        ) : (
          <Fragment key={i}>{renderInline(n.children)}</Fragment>
        )
      }
    }
  })
}

function CodeBlock({ lang, text }: { lang: string; text: string }) {
  const toast = useToast()
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])

  const copy = () => {
    const fallback = () => toast('Could not copy. Select the text and press Ctrl+C.')
    if (!navigator.clipboard) return fallback()
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      clearTimeout(timer.current)
      timer.current = setTimeout(() => setCopied(false), 1500)
    }, fallback)
  }

  return (
    <div className="md-code">
      <div className="md-code-bar">
        <span>{lang}</span>
        <button type="button" className="md-copy" onClick={copy}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre tabIndex={0}>
        <code>{text}</code>
      </pre>
    </div>
  )
}
