import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const src = path.resolve(import.meta.dirname, '..')
const self = path.resolve(import.meta.filename)
// Built from pieces so this file does not match its own pattern.
const primitive = new RegExp(
  `['"](@${'radix'}-ui/|${'radix'}-ui['"/]|@${'base'}-ui)`,
)

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = path.join(dir, e.name)
    if (e.isDirectory()) return sources(full)
    return /\.(ts|tsx)$/.test(e.name) ? [full] : []
  })
}

describe('components/ui boundary', () => {
  it('lets only the shadcn primitives import the Radix or Base UI packages', () => {
    const ui = path.join(src, 'components', 'ui')
    const offenders = sources(src).filter(
      (file) =>
        file !== self &&
        !file.startsWith(ui + path.sep) &&
        primitive.test(readFileSync(file, 'utf8')),
    )
    expect(offenders).toEqual([])
  })
})
