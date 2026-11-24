import { useEffect, useState } from 'react'
import { ChevronDown, Radar } from 'lucide-react'
import { motion, AnimatePresence, useReducedMotion } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import type { DiscoveryIface, DiscoveryKind, DiscoveryMode, DiscoveryOverrides } from '@/data/types'
import { SectionHeader } from '@/components/SectionHeader'
import { SegmentedControl } from '@/components/SegmentedControl'
import { cn } from '@/lib/utils'

/**
 * ClientDiscoveryPanel: configuración por unidad y por red lógica de OpenWrt
 * (o dispositivo Linux de respaldo) de si los vecinos aprendidos ahí (ARP/NDP)
 * cuentan como clientes. Automático ignora la red por la que la unidad sale a
 * Internet y activa el resto.
 */
export function ClientDiscoveryPanel({
  routerId,
  discovery,
  isAdmin,
  onUpdated,
}: {
  routerId: string
  discovery: DiscoveryIface[]
  isAdmin: boolean
  onUpdated?: () => void
}) {
  const { t } = useTranslation()
  const reduce = useReducedMotion()
  const [open, setOpen] = useState(true)
  const [rows, setRows] = useState<DiscoveryIface[]>(discovery)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setRows(discovery)
  }, [discovery])

  if (!rows || rows.length === 0) return null

  async function handleModeChange(kind: DiscoveryKind, name: string, mode: DiscoveryMode) {
    if (!isAdmin || busy) return
    const prevRows = rows
    const nextRows = rows.map((r) =>
      r.kind === kind && r.name === name
        ? { ...r, mode, effective: mode === 'on' ? true : mode === 'off' ? false : r.auto }
        : r,
    )
    setRows(nextRows)
    setError(null)
    setBusy(true)

    const networks: Record<string, string> = {}
    const devices: Record<string, string> = {}
    for (const r of nextRows) {
      if (r.mode === 'on' || r.mode === 'off') {
        if (r.kind === 'network') {
          networks[r.name] = r.mode
        } else {
          devices[r.name] = r.mode
        }
      }
    }
    const discovery_overrides: DiscoveryOverrides = { networks, devices }

    try {
      const res = await fetch(`/api/config/routers/${encodeURIComponent(routerId)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ discovery_overrides }),
      })
      if (!res.ok) {
        setRows(prevRows)
        setError(t('routerDetail.discovery.saveError'))
        return
      }
      onUpdated?.()
    } catch {
      setRows(prevRows)
      setError(t('routerDetail.discovery.saveError'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section
      className="rounded-2xl border border-border bg-surface p-5 md:p-6 lg:col-span-12"
      data-testid="client-discovery-panel"
    >
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between gap-3"
        aria-expanded={open}
        aria-label={t('routerDetail.discovery.toggleAria', { count: rows.length })}
      >
        <SectionHeader title={t('routerDetail.discovery.title')} />
        <div className="flex items-center gap-2">
          <span className="rounded-full bg-accent/10 px-2 py-0.5 font-mono text-caption font-semibold text-accent">
            {rows.length}
          </span>
          <ChevronDown
            className={cn('h-4 w-4 text-text-muted transition-transform duration-200', open && 'rotate-180')}
            strokeWidth={1.75}
          />
        </div>
      </button>

      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            key="discovery-body"
            initial={{ height: reduce ? 'auto' : 0, opacity: reduce ? 1 : 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: reduce ? 'auto' : 0, opacity: reduce ? 1 : 0 }}
            transition={{ duration: 0.2, ease: 'easeInOut' }}
            className="overflow-hidden"
          >
            <p className="mt-2 text-sm text-text-secondary">
              {t('routerDetail.discovery.description')}
            </p>
            {error && (
              <p role="alert" className="mt-2 text-sm font-medium text-danger">
                {error}
              </p>
            )}
            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-border/60 text-[10px] font-semibold uppercase tracking-[0.06em] text-text-muted">
                    <th className="pb-2 pr-4">{t('routerDetail.discovery.network')}</th>
                    <th className="pb-2 pr-4">{t('routerDetail.discovery.neighbours')}</th>
                    <th className="pb-2 text-right">{t('routerDetail.discovery.mode')}</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => {
                    const autoLabel = t(
                      row.auto ? 'routerDetail.discovery.autoOn' : 'routerDetail.discovery.autoOff',
                    )
                    const readOnlyLabel =
                      row.mode === 'auto'
                        ? autoLabel
                        : row.mode === 'on'
                          ? t('routerDetail.discovery.on')
                          : t('routerDetail.discovery.off')
                    return (
                      <tr key={`${row.kind}:${row.name}`} className="border-b border-border/30 last:border-0">
                        <td className="py-2.5 pr-4">
                          <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5">
                            {row.kind === 'network' ? (
                              <>
                                <span className="inline-flex items-center gap-1.5 font-semibold text-text-primary">
                                  <Radar className="h-3.5 w-3.5 shrink-0 text-text-muted" strokeWidth={1.75} />
                                  {row.name}
                                </span>
                                {row.devices && row.devices.length > 0 && (
                                  <span className="font-mono text-caption text-text-muted">
                                    · {row.devices.join(', ')}
                                  </span>
                                )}
                                {row.up === false && (
                                  <span className="rounded bg-border/50 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-text-muted">
                                    {t('routerDetail.discovery.down')}
                                  </span>
                                )}
                              </>
                            ) : (
                              <>
                                <span className="inline-flex items-center gap-1.5 font-mono text-mono-sm font-semibold text-text-primary">
                                  <Radar className="h-3.5 w-3.5 shrink-0 text-text-muted" strokeWidth={1.75} />
                                  {row.name}
                                </span>
                                <span className="rounded bg-border/50 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-text-muted">
                                  {t('routerDetail.discovery.deviceLabel')}
                                </span>
                              </>
                            )}
                          </div>
                        </td>
                        <td className="py-2.5 pr-4 font-mono text-mono-sm text-text-secondary">
                          {row.neighbours}
                        </td>
                        <td className="py-2.5 text-right">
                          {isAdmin ? (
                            <SegmentedControl<DiscoveryMode>
                              size="sm"
                              value={row.mode}
                              onChange={(val) => void handleModeChange(row.kind, row.name, val)}
                              ariaLabel={t('routerDetail.discovery.modeAria', { name: row.name })}
                              options={[
                                { value: 'auto', label: autoLabel },
                                { value: 'on', label: t('routerDetail.discovery.on') },
                                { value: 'off', label: t('routerDetail.discovery.off') },
                              ]}
                            />
                          ) : (
                            <span className="font-mono text-caption text-text-secondary">
                              {readOnlyLabel}
                            </span>
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </section>
  )
}
