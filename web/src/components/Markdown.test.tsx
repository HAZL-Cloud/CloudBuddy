import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '../test/utils'
import { Markdown } from './Markdown'

// Shaped like a real agent reply (IDs and addresses are documentation examples).
const AGENT_REPLY = `**What's done:**
- **Port 8501 needs no change.** The security group already allows it.
- **The VM is ready.**

**To finish, either:**
1. **Allow SSH in the sandbox.** Add \`203.0.113.10:22\` to the network settings.
2. **Run it yourself.** Connect with:
   \`\`\`
   aws ec2-instance-connect ssh --instance-id i-0123456789abcdef0
   \`\`\`
   Then on the VM:
   \`\`\`
   sudo dnf install -y docker
   \`\`\`

Its data is kept in \`/opt/memos\`.

- **This is a production server.**
- **Port 8501 is open to everyone.**`

const show = (text: string) => renderWithProviders(<Markdown text={text} />).container.querySelector('.md') as HTMLElement

afterEach(() => vi.unstubAllGlobals())

describe('Markdown', () => {
  it('shows an agent reply as lists, bold text and code blocks', () => {
    const md = show(AGENT_REPLY)
    expect(within(md).getByText("What's done:").tagName).toBe('STRONG')
    expect(md.querySelectorAll('ul > li')).toHaveLength(4)
    const steps = md.querySelectorAll('ol > li')
    expect(steps).toHaveLength(2)
    expect(steps[1]?.querySelectorAll('pre')).toHaveLength(2)
    expect(within(md).getByText(/aws ec2-instance-connect ssh/).tagName).toBe('CODE')
    expect(within(md).getAllByRole('button', { name: 'Copy' })).toHaveLength(2)
    expect(within(md).getByText('/opt/memos').tagName).toBe('CODE')
    expect(md.textContent).not.toContain('**')
  })

  it('opens web links in a new tab without giving the page access to CloudBuddy', () => {
    const md = show('Open [the site](https://example.com/a) or http://203.0.113.10:8501.')
    const links = within(md).getAllByRole('link')
    expect(links.map((a) => a.getAttribute('href'))).toEqual(['https://example.com/a', 'http://203.0.113.10:8501/'])
    for (const a of links) {
      expect(a).toHaveAttribute('target', '_blank')
      expect(a).toHaveAttribute('rel', 'noopener noreferrer')
    }
  })

  it('shows links to anything but the web as plain text', () => {
    const md = show('[click](javascript:alert(1)) [data](data:text/html,hi) [file](internal/server.go) [mail](mailto:a@b.c)')
    expect(within(md).queryAllByRole('link')).toHaveLength(0)
    expect(md.textContent).toBe('click data file mail')
  })

  it('shows HTML in agent text as text', () => {
    const md = show('<script>alert(1)</script> <img src=x onerror=alert(1)> **<b>hi</b>**')
    expect(md.querySelector('script, img, b')).toBeNull()
    expect(md.textContent).toBe('<script>alert(1)</script> <img src=x onerror=alert(1)> <b>hi</b>')
  })

  it('starts headings at h3, below the chat title', () => {
    const md = show('# One\n## Two\n### Three\n###### Six')
    expect(md.querySelector('h1, h2')).toBeNull()
    expect([...md.querySelectorAll('h3, h4, h5, h6')].map((h) => h.tagName)).toEqual(['H3', 'H4', 'H5', 'H6'])
  })

  it('keeps line breaks, numbered starts and table alignment', () => {
    const md = show('one\ntwo\n\n3. three\n\n| a | b |\n|---|--:|\n| 1 | 2 |')
    expect(md.querySelector('p')?.querySelectorAll('br')).toHaveLength(1)
    expect(md.querySelector('ol')).toHaveAttribute('start', '3')
    const region = within(md).getByRole('region', { name: 'Table' })
    expect(within(region).getByText('2')).toHaveClass('al-right')
  })

  it('copies a code block and says so', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    const md = show('```sh\necho hi\n```')
    expect(within(md).getByText('sh')).toBeInTheDocument()
    await userEvent.click(within(md).getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith('echo hi')
    expect(await within(md).findByRole('button', { name: 'Copied' })).toBeInTheDocument()
  })

  it('says how to copy by hand when the clipboard is not available', async () => {
    vi.stubGlobal('navigator', { ...navigator, clipboard: undefined })
    const md = show('```\necho hi\n```')
    await userEvent.click(within(md).getByRole('button', { name: 'Copy' }))
    expect(screen.getByRole('status')).toHaveTextContent('Could not copy. Select the text and press Ctrl+C.')
  })
})
