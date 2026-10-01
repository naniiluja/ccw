import { useQueryClient } from '@tanstack/react-query'
import { LogOutIcon } from 'lucide-react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { isApiError } from '@/api/client'
import { sessionQuery, signOut, useSession } from '@/api/session'
import { routeTable } from '@/app/routes'
import { AppSidebar } from '@/components/app/app-sidebar'
import { CommandPalette } from '@/components/app/command-palette'
import { ThemeToggle } from '@/components/app/theme-toggle'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from '@/components/ui/sidebar'

function Crumbs() {
  const { pathname } = useLocation()
  const here = routeTable.find((r) => r.path === pathname)
  const title = here?.title ?? 'Không tìm thấy trang'
  return (
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem className="hidden sm:inline-flex">
          <BreadcrumbLink asChild>
            <Link to="/">ccw</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator className="hidden sm:block" />
        <BreadcrumbItem>
          <BreadcrumbPage>{title}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  )
}

function SignOutButton() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  async function onClick() {
    try {
      await signOut()
    } catch (e) {
      toast.error(isApiError(e) ? e.message : 'Không đăng xuất được.')
      return
    }
    queryClient.setQueryData(sessionQuery.queryKey, (s) =>
      s ? { ...s, authenticated: false } : s,
    )
    queryClient.removeQueries({
      predicate: (q) => q.queryKey[0] !== sessionQuery.queryKey[0],
    })
    await navigate('/login')
  }
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label="Đăng xuất"
      onClick={() => void onClick()}
    >
      <LogOutIcon aria-hidden="true" />
    </Button>
  )
}

export function AppShell() {
  const session = useSession()
  return (
    <SidebarProvider>
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:shadow-md"
      >
        Bỏ qua tới nội dung
      </a>
      <AppSidebar />
      <SidebarInset className="min-w-0">
        <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background/85 px-3 backdrop-blur sm:px-4">
          <SidebarTrigger aria-label="Mở hoặc thu gọn thanh bên" />
          <Separator orientation="vertical" className="mr-1 h-4" />
          <Crumbs />
          <div className="ml-auto flex items-center gap-1">
            <CommandPalette />
            <ThemeToggle />
            {session.data?.authRequired ? <SignOutButton /> : null}
          </div>
        </header>
        <main
          id="main"
          tabIndex={-1}
          className="mx-auto w-full max-w-7xl flex-1 p-4 outline-none sm:p-6"
        >
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
