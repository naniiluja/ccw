import {
  ActivityIcon,
  GaugeIcon,
  GitCompareArrowsIcon,
  KeyRoundIcon,
  LayoutDashboardIcon,
  type LucideIcon,
  PlugZapIcon,
  FilterIcon,
  SettingsIcon,
  TriangleAlertIcon,
  UsersIcon,
} from 'lucide-react'
import type { ComponentType } from 'react'

export interface AppRoute {
  path: string
  /** Folder under src/pages that holds the page. */
  page: string
  /** Vietnamese title for the sidebar, breadcrumb and command palette. */
  title: string
  icon?: LucideIcon
  /** Extra search terms for the command palette, such as the English feature name. */
  keywords?: string[]
  /** Whether the route gets a sidebar entry. */
  nav: boolean
  /** The page renders inside the shell (login and the not-found page do not need the sidebar). */
  shell: boolean
  load: () => Promise<{ default: ComponentType }>
}

// The single route table. The router, the sidebar, the breadcrumb and the
// command palette are all generated from it. A feature task replaces the body
// of its src/pages/<page>/index.tsx and leaves this file alone.
export const routeTable: AppRoute[] = [
  {
    path: '/',
    page: 'overview',
    title: 'Tổng quan',
    keywords: ['overview','dashboard','home'],
    icon: LayoutDashboardIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/overview'),
  },
  {
    path: '/accounts',
    page: 'accounts',
    title: 'Tài khoản',
    keywords: ['accounts','account','credential'],
    icon: UsersIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/accounts'),
  },
  {
    path: '/providers',
    page: 'providers',
    title: 'Nhà cung cấp',
    keywords: ['providers','provider','model','rotation'],
    icon: PlugZapIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/providers'),
  },
  {
    path: '/keys',
    page: 'keys',
    title: 'Khóa API',
    keywords: ['keys','api key','token','rpm'],
    icon: KeyRoundIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/keys'),
  },
  {
    path: '/quota',
    page: 'quota',
    title: 'Hạn mức',
    keywords: ['quota','limit','reset'],
    icon: GaugeIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/quota'),
  },
  {
    path: '/usage',
    page: 'usage',
    title: 'Lượng dùng',
    keywords: ['usage','token','chart'],
    icon: ActivityIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/usage'),
  },
  {
    path: '/errors',
    page: 'errors',
    title: 'Lỗi upstream',
    keywords: ['errors','error','upstream','review'],
    icon: TriangleAlertIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/errors'),
  },
  {
    path: '/drift',
    page: 'drift',
    title: 'Thay đổi shape',
    keywords: ['drift','shape','schema'],
    icon: GitCompareArrowsIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/drift'),
  },
  {
    path: '/filters',
    page: 'filters',
    title: 'Bộ lọc',
    keywords: ['filters','filter','blacklist','field'],
    icon: FilterIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/filters'),
  },
  {
    path: '/settings',
    page: 'settings',
    title: 'Cài đặt',
    keywords: ['settings','config','web search','timezone'],
    icon: SettingsIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/settings'),
  },
  {
    path: '/login',
    page: 'login',
    title: 'Đăng nhập',
    nav: false,
    shell: false,
    load: () => import('@/pages/login'),
  },
  {
    path: '*',
    page: 'not-found',
    title: 'Không tìm thấy trang',
    nav: false,
    shell: true,
    load: () => import('@/pages/not-found'),
  },
]
