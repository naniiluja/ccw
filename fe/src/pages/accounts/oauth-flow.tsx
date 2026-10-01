import { ExternalLinkIcon, Loader2Icon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { isApiError } from '@/api/client'
import {
  type CodeStart,
  type DeviceStart,
  finishOAuth,
  isDeviceStart,
  pollDevice,
  startOAuth,
} from '@/api/oauth'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { text } from './strings'

const t = text.dialog
const message = (e: unknown) => (e instanceof Error ? e.message : text.unknownError)

interface Props {
  provider: string
  onDone: () => void
}

// OAuthFlow runs one sign-in: it starts it, then either takes the pasted code
// or polls the device code. What the server hands out for the sign-in (state,
// device code) lives in this component only, so closing the dialog drops it
// and stops the polling.
export function OAuthFlow({ provider, onDone }: Props) {
  const [label, setLabel] = useState('')
  const [begun, setBegun] = useState<CodeStart | DeviceStart | null>(null)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string>()

  async function start() {
    setStarting(true)
    setError(undefined)
    try {
      setBegun(await startOAuth(provider, label.trim() || undefined))
    } catch (e) {
      setError(message(e))
    } finally {
      setStarting(false)
    }
  }

  if (begun && isDeviceStart(begun)) {
    return (
      <DeviceStep
        provider={provider}
        device={begun}
        label={label.trim()}
        onDone={onDone}
        onRestart={() => setBegun(null)}
      />
    )
  }
  if (begun) {
    return <CodeStep provider={provider} start={begun} label={label.trim()} onDone={onDone} />
  }
  return (
    <div className="flex flex-col gap-4">
      <Field>
        <FieldLabel htmlFor="oauth-label">{t.label}</FieldLabel>
        <Input
          id="oauth-label"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          autoComplete="off"
        />
      </Field>
      {error ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <Button onClick={() => void start()} disabled={starting}>
        {starting ? <Loader2Icon className="animate-spin" aria-hidden="true" /> : null}
        {starting ? t.starting : t.start}
      </Button>
    </div>
  )
}

function CodeStep({
  provider,
  start,
  label,
  onDone,
}: {
  provider: string
  start: CodeStart
  label: string
  onDone: () => void
}) {
  const [input, setInput] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  async function finish(e: React.FormEvent) {
    e.preventDefault()
    if (!input.trim()) {
      setError(t.pasteRequired)
      return
    }
    setBusy(true)
    setError(undefined)
    const pasted = input.trim()
    // The pasted code is used once; it is dropped from state as it is sent.
    setInput('')
    try {
      await finishOAuth(provider, { state: start.state, input: pasted, label: label || undefined })
      onDone()
    } catch (err) {
      setInput(pasted)
      setError(message(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form noValidate onSubmit={(e) => void finish(e)} className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">{t.openHint}</p>
      <Button asChild variant="outline">
        <a href={start.url} target="_blank" rel="noopener noreferrer">
          {t.open}
          <ExternalLinkIcon aria-hidden="true" />
        </a>
      </Button>
      <Field data-invalid={error ? true : undefined}>
        <FieldLabel htmlFor="oauth-paste">{t.paste}</FieldLabel>
        <Textarea
          id="oauth-paste"
          value={input}
          rows={3}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={error ? true : undefined}
          onChange={(e) => {
            setInput(e.target.value)
            setError(undefined)
          }}
        />
        <FieldError>{error}</FieldError>
      </Field>
      <Button type="submit" disabled={busy}>
        {busy ? <Loader2Icon className="animate-spin" aria-hidden="true" /> : null}
        {busy ? t.finishing : t.finish}
      </Button>
    </form>
  )
}

function DeviceStep({
  provider,
  device,
  label,
  onDone,
  onRestart,
}: {
  provider: string
  device: DeviceStart
  label: string
  onDone: () => void
  onRestart: () => void
}) {
  const [expired, setExpired] = useState(false)
  const [error, setError] = useState<string>()
  const done = useRef(onDone)
  useEffect(() => {
    done.current = onDone
  })

  useEffect(() => {
    const ctrl = new AbortController()
    const deadline = Date.now() + device.expiresIn * 1000
    const wait = Math.max(device.interval, 1) * 1000
    let timer: ReturnType<typeof setTimeout>
    async function tick() {
      if (Date.now() >= deadline) {
        setExpired(true)
        return
      }
      try {
        const r = await pollDevice(provider, device.deviceCode, label || undefined, ctrl.signal)
        if (ctrl.signal.aborted) return
        if (r.status === 'done') {
          done.current()
          return
        }
      } catch (e) {
        if (ctrl.signal.aborted) return
        // A refusal (4xx) will not change; a network or server hiccup may.
        if (isApiError(e) && e.status >= 400 && e.status < 500) {
          setError(e.message)
          return
        }
      }
      timer = setTimeout(() => void tick(), wait)
    }
    timer = setTimeout(() => void tick(), wait)
    return () => {
      ctrl.abort()
      clearTimeout(timer)
    }
  }, [provider, device, label])

  const ended = expired || error !== undefined
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">{t.deviceHint}</p>
      <div className="rounded-lg border bg-muted/40 p-4 text-center">
        <p className="text-xs text-muted-foreground">{t.deviceCode}</p>
        <p className="mt-1 font-mono text-2xl font-semibold tracking-widest select-all">
          {device.userCode}
        </p>
      </div>
      <Button asChild variant="outline">
        <a href={device.verificationUri} target="_blank" rel="noopener noreferrer">
          {t.deviceOpen}
          <ExternalLinkIcon aria-hidden="true" />
        </a>
      </Button>
      {ended ? (
        <>
          <Alert variant="destructive" role="alert">
            <AlertDescription>{error ?? t.expired}</AlertDescription>
          </Alert>
          <Button variant="outline" onClick={onRestart}>
            {t.restart}
          </Button>
        </>
      ) : (
        <FieldDescription role="status" className="flex items-center gap-2">
          <Loader2Icon className="size-4 animate-spin" aria-hidden="true" />
          {t.waiting}
        </FieldDescription>
      )}
    </div>
  )
}
