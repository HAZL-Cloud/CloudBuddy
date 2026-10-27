import { splitCode } from '../format'

/** Agent text with `backtick` spans shown as code. Every piece is plain text inside a React element. */
export function InlineCode({ text }: { text: string }) {
  return (
    <>
      {splitCode(text).map((part, i) => (part.code ? <code key={i}>{part.text}</code> : part.text))}
    </>
  )
}
