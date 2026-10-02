import { SearchIcon } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { routeTable } from '@/app/routes'
import { themeChoices } from '@/components/app/theme-choices'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Kbd } from '@/components/ui/kbd'

// The palette opens with Cmd/Ctrl+K only. A single-character shortcut would
// fire while someone types in a field (WCAG 2.1.4), so there is none.
export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const { setTheme } = useTheme()

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key.toLowerCase() === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  function run(action: () => void) {
    setOpen(false)
    action()
  }

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="text-muted-foreground"
        onClick={() => setOpen(true)}
        aria-label="Mở bảng lệnh"
      >
        <SearchIcon aria-hidden="true" />
        <span className="hidden sm:inline">Tìm trang hoặc lệnh</span>
        <Kbd className="hidden sm:inline-flex">Ctrl K</Kbd>
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          showCloseButton={false}
          className="top-1/3 translate-y-0 overflow-hidden p-0"
        >
              <DialogHeader className="sr-only">
                <DialogTitle>Bảng lệnh</DialogTitle>
                <DialogDescription>
                  Chuyển nhanh tới một trang hoặc đổi giao diện.
                </DialogDescription>
              </DialogHeader>
              <Command>
            <CommandInput placeholder="Gõ tên trang hoặc lệnh" />
            <CommandList>
              <CommandEmpty>Không có kết quả.</CommandEmpty>
              <CommandGroup heading="Trang">
                {routeTable
                  .filter((r) => r.nav)
                  .map(({ path, title, keywords, icon: Icon }) => (
                    <CommandItem
                      key={path}
                      value={title}
                      keywords={keywords}
                      onSelect={() => run(() => void navigate(path))}
                    >
                      {Icon ? <Icon aria-hidden="true" /> : null}
                      {title}
                    </CommandItem>
                  ))}
              </CommandGroup>
          <CommandGroup heading="Giao diện">
            {themeChoices.map(({ value, label, icon: Icon }) => (
              <CommandItem
                key={value}
                value={`Giao diện ${label}`}
                onSelect={() => run(() => setTheme(value))}
              >
                <Icon aria-hidden="true" />
                {`Giao diện: ${label}`}
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
          </Command>
        </DialogContent>
      </Dialog>
    </>
  )
}
