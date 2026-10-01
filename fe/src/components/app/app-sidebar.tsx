import { NetworkIcon } from 'lucide-react'
import { NavLink, useLocation } from 'react-router'
import { routeTable } from '@/app/routes'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@/components/ui/sidebar'

// The sidebar is generated from the route table: a route with nav: true gets
// an entry. On a narrow screen the sidebar component renders itself as a Sheet.
export function AppSidebar() {
  const { pathname } = useLocation()
  const { setOpenMobile } = useSidebar()
  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild tooltip="ccw">
              <NavLink to="/" onClick={() => setOpenMobile(false)}>
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <NetworkIcon className="size-4" aria-hidden="true" />
                </div>
                <div className="grid flex-1 text-left leading-tight">
                  <span className="truncate font-semibold">ccw</span>
                  <span className="truncate text-xs text-muted-foreground">
                    Credential proxy
                  </span>
                </div>
              </NavLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <nav aria-label="Điều hướng chính">
              <SidebarMenu>
                {routeTable
                  .filter((r) => r.nav)
                  .map(({ path, title, icon: Icon }) => (
                    <SidebarMenuItem key={path}>
                      <SidebarMenuButton
                        asChild
                        tooltip={title}
                        isActive={
                          path === '/' ? pathname === '/' : pathname.startsWith(path)
                        }
                      >
                        <NavLink to={path} onClick={() => setOpenMobile(false)}>
                          {Icon ? <Icon aria-hidden="true" /> : null}
                          <span>{title}</span>
                        </NavLink>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
              </SidebarMenu>
            </nav>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}
