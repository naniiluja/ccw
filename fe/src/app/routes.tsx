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
    icon: LayoutDashboardIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/overview'),
  },
  {
    path: '/accounts',
    page: 'accounts',
    title: 'Tài khoản',
    icon: UsersIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/accounts'),
  },
  {
    path: '/providers',
    page: 'providers',
    title: 'Nhà cung cấp',
    icon: PlugZapIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/providers'),
  },
  {
    path: '/keys',
    page: 'keys',
    title: 'Khóa API',
    icon: KeyRoundIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/keys'),
  },
  {
    path: '/quota',
    page: 'quota',
    title: 'Hạn mức',
    icon: GaugeIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/quota'),
  },
  {
    path: '/usage',
    page: 'usage',
    title: 'Lượng dùng',
    icon: ActivityIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/usage'),
  },
  {
    path: '/errors',
    page: 'errors',
    title: 'Lỗi upstream',
    icon: TriangleAlertIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/errors'),
  },
  {
    path: '/drift',
    page: 'drift',
    title: 'Thay đổi shape',
    icon: GitCompareArrowsIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/drift'),
  },
  {
    path: '/filters',
    page: 'filters',
    title: 'Bộ lọc',
    icon: FilterIcon,
    nav: true,
    shell: true,
    load: () => import('@/pages/filters'),
  },
  {
    path: '/settings',
    page: 'settings',
    title: 'Cài đặt',
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
