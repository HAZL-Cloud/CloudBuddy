import { describe, expect, it } from 'vitest'
import tokens from './styles/tokens.css?raw'
import { applyTheme, storedTheme, THEME_KEY } from './theme'

/** The custom properties declared in the first rule whose selector matches, as name → value. */
function block(selector: RegExp): Map<string, string> {
  const match = tokens.match(new RegExp(`${selector.source}\\s*\\{([^}]*)\\}`, selector.flags))
  if (!match) throw new Error(`no rule matching ${selector}`)
  return new Map([...match[1].matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]))
}

describe('the theme choice', () => {
  it('reads what was saved, and treats anything else as the computer setting', () => {
    expect(storedTheme()).toBe('system')
    window.localStorage.setItem(THEME_KEY, '"dark"')
    expect(storedTheme()).toBe('dark')
    window.localStorage.setItem(THEME_KEY, '"light"')
    expect(storedTheme()).toBe('light')
    window.localStorage.setItem(THEME_KEY, '"blue"')
    expect(storedTheme()).toBe('system')
    window.localStorage.setItem(THEME_KEY, 'not json')
    expect(storedTheme()).toBe('system')
  })

  it('sets data-theme for light and dark, and removes it to follow the computer', () => {
    applyTheme('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
    applyTheme('light')
    expect(document.documentElement.dataset.theme).toBe('light')
    applyTheme('system')
    expect(document.documentElement.dataset.theme).toBeUndefined()
  })
})

describe('the dark colours', () => {
  const light = block(/^:root/m)
  const chosen = block(/:root\[data-theme="dark"\]/)
  const system = block(/:root:not\(\[data-theme="light"\]\)/)

  it('are the same whether you pick Dark or the computer is set to dark', () => {
    expect(Object.fromEntries(chosen)).toEqual(Object.fromEntries(system))
  })

  it('cover every colour the light theme sets', () => {
    const colours = [...light.keys()].filter((name) => !name.startsWith('--f-'))
    expect(colours.filter((name) => !chosen.has(name))).toEqual([])
  })
})
