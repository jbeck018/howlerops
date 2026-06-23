import { Bell, Play, Trash2 } from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useAlerts } from '@/hooks/use-alerts'
import { toast } from '@/hooks/use-toast'
import { type Comparator, saveAlert } from '@/lib/alert-api'

/**
 * AlertListPanel manages standalone time-series alerts: enable/disable, check
 * now, delete, and a compact form to create a threshold alert.
 */
export function AlertListPanel() {
  const { alerts, loading, error, refresh, toggle, remove, check } = useAlerts()

  const onCheck = async (id: string, name: string) => {
    try {
      const res = await check(id)
      toast({
        title: `Alert: ${name}`,
        description: res.fired ? res.message || 'Firing now.' : 'Not firing.',
        variant: res.fired ? 'destructive' : undefined,
      })
    } catch (err) {
      toast({ title: 'Check failed', description: err instanceof Error ? err.message : 'Error', variant: 'destructive' })
    }
  }

  return (
    <div className="space-y-4 p-3">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="flex items-center gap-2 text-sm">
            <Bell className="h-4 w-4" /> Alerts
          </CardTitle>
          <Button size="sm" variant="ghost" onClick={() => void refresh()}>
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="space-y-2">
          {loading && <p className="text-xs text-muted-foreground">Loading…</p>}
          {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
          {!loading && alerts.length === 0 && <p className="text-xs text-muted-foreground">No alerts yet.</p>}
          {alerts.map((a) => (
            <div key={a.id} className="flex items-center gap-2 rounded border p-2">
              <Switch checked={a.enabled} onCheckedChange={(c) => void toggle(a.id, c)} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{a.name}</div>
                <div className="truncate text-xs text-muted-foreground">
                  {ruleSummary(a.rule)}
                  {a.lastFiredAt && ` · last fired ${new Date(a.lastFiredAt).toLocaleString()}`}
                </div>
              </div>
              <Button size="sm" variant="ghost" onClick={() => void onCheck(a.id, a.name)} title="Check now">
                <Play className="h-4 w-4" />
              </Button>
              <Button size="sm" variant="ghost" onClick={() => void remove(a.id)} title="Delete">
                <Trash2 className="h-4 w-4 text-destructive" />
              </Button>
            </div>
          ))}
        </CardContent>
      </Card>

      <CreateThresholdAlert onCreated={() => void refresh()} />
    </div>
  )
}

function ruleSummary(rule: { threshold?: { comparator: string; value: number }; anomaly?: unknown; forecast?: { comparator: string; value: number } }): string {
  if (rule.threshold) return `latest ${rule.threshold.comparator} ${rule.threshold.value}`
  if (rule.anomaly) return 'anomaly detection'
  if (rule.forecast) return `forecast ${rule.forecast.comparator} ${rule.forecast.value}`
  return 'rule'
}

function CreateThresholdAlert({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('')
  const [connectionId, setConnectionId] = useState('')
  const [sql, setSql] = useState('')
  const [valueColumn, setValueColumn] = useState('')
  const [comparator, setComparator] = useState<Comparator>('gt')
  const [value, setValue] = useState('')
  const [saving, setSaving] = useState(false)

  const canSave = name.trim() && connectionId.trim() && sql.trim() && value.trim()

  const onSubmit = async () => {
    if (!canSave) return
    setSaving(true)
    try {
      await saveAlert({
        name,
        connectionId,
        sql,
        valueColumn: valueColumn || undefined,
        enabled: true,
        rule: { threshold: { comparator, value: Number(value) } },
      })
      toast({ title: 'Alert created', description: name })
      setName('')
      setSql('')
      setValue('')
      onCreated()
    } catch (err) {
      toast({ title: 'Failed to create alert', description: err instanceof Error ? err.message : 'Error', variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">New threshold alert</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="grid grid-cols-2 gap-2">
          <div className="space-y-1">
            <Label htmlFor="al-name" className="text-xs">Name</Label>
            <Input id="al-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="al-conn" className="text-xs">Connection ID</Label>
            <Input id="al-conn" value={connectionId} onChange={(e) => setConnectionId(e.target.value)} />
          </div>
        </div>
        <div className="space-y-1">
          <Label htmlFor="al-sql" className="text-xs">Query (time + value column)</Label>
          <Input id="al-sql" value={sql} onChange={(e) => setSql(e.target.value)} placeholder="SELECT day, revenue FROM sales ORDER BY day" />
        </div>
        <div className="grid grid-cols-3 gap-2">
          <div className="space-y-1">
            <Label htmlFor="al-col" className="text-xs">Value column</Label>
            <Input id="al-col" value={valueColumn} onChange={(e) => setValueColumn(e.target.value)} placeholder="auto" />
          </div>
          <div className="space-y-1">
            <Label htmlFor="al-cmp" className="text-xs">When latest</Label>
            <Select value={comparator} onValueChange={(v) => setComparator(v as Comparator)}>
              <SelectTrigger id="al-cmp"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="gt">&gt;</SelectItem>
                <SelectItem value="gte">&ge;</SelectItem>
                <SelectItem value="lt">&lt;</SelectItem>
                <SelectItem value="lte">&le;</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label htmlFor="al-val" className="text-xs">Threshold</Label>
            <Input id="al-val" type="number" value={value} onChange={(e) => setValue(e.target.value)} />
          </div>
        </div>
        <Button size="sm" onClick={() => void onSubmit()} disabled={!canSave || saving}>
          {saving ? 'Creating…' : 'Create alert'}
        </Button>
      </CardContent>
    </Card>
  )
}
