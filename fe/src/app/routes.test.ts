import { existsSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { routeTable } from './routes'

describe('route table', () => {
  it('declares exactly the 12 routes of the app', () => {
    expect(routeTable.map((r) => r.path)).toEqual([
      '/',
      '/accounts',
      '/providers',
      '/keys',
      '/quota',
      '/usage',
      '/errors',
      '/drift',
      '/filters',
      '/settings',
      '/login',
      '*',
    ])
  })

  it('has a page file for every route', () => {
    for (const route of routeTable) {
      const file = path.resolve(
        import.meta.dirname,
        '..',
        'pages',
        route.page,
        'index.tsx',
      )
      expect(existsSync(file), `${route.path} -> ${file}`).toBe(true)
    }
  })

  it('puts only the pages with a sidebar entry in the navigation', () => {
    const nav = routeTable.filter((r) => r.nav).map((r) => r.path)
    expect(nav).not.toContain('/login')
    expect(nav).not.toContain('*')
    expect(nav).toHaveLength(10)
  })
})
