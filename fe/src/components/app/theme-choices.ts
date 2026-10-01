import { MonitorIcon, MoonIcon, SunIcon } from 'lucide-react'

export const themeChoices = [
  { value: 'light', label: 'Sáng', icon: SunIcon },
  { value: 'dark', label: 'Tối', icon: MoonIcon },
  { value: 'system', label: 'Theo hệ thống', icon: MonitorIcon },
] as const
