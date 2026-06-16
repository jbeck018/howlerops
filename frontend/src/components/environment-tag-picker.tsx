import { Check, Plus, X } from "lucide-react"
import { useState } from "react"
import { useShallow } from "zustand/react/shallow"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { cn } from "@/lib/utils"
import { useConnectionStore } from "@/store/connection-store"

interface EnvironmentTagPickerProps {
  connectionId: string
  environments: string[]
  /** Compact mode renders just a small "+"/edit trigger without inline chips. */
  compact?: boolean
  className?: string
}

/**
 * Inline environment tagger for a connection — assign/remove environment tags
 * (which drive the connection folders) without opening the edit dialog. Reads
 * the known environments from the store and lets you create new ones on the fly.
 */
export function EnvironmentTagPicker({
  connectionId,
  environments,
  compact = false,
  className,
}: EnvironmentTagPickerProps) {
  const { availableEnvironments, addEnvironmentToConnection, removeEnvironmentFromConnection } =
    useConnectionStore(
      useShallow((state) => ({
        availableEnvironments: state.availableEnvironments,
        addEnvironmentToConnection: state.addEnvironmentToConnection,
        removeEnvironmentFromConnection: state.removeEnvironmentFromConnection,
      }))
    )

  const [open, setOpen] = useState(false)
  const [newEnv, setNewEnv] = useState("")

  const assigned = new Set(environments)

  const toggle = (env: string) => {
    if (assigned.has(env)) {
      removeEnvironmentFromConnection(connectionId, env)
    } else {
      addEnvironmentToConnection(connectionId, env)
    }
  }

  const addNew = () => {
    const value = newEnv.trim()
    if (!value) return
    addEnvironmentToConnection(connectionId, value)
    setNewEnv("")
  }

  return (
    <div className={cn("flex flex-wrap items-center gap-1", className)}>
      {!compact &&
        environments.map((env) => (
          <Badge key={env} variant="secondary" className="gap-1 pr-1 text-xs">
            {env}
            <button
              type="button"
              aria-label={`Remove ${env}`}
              className="rounded hover:bg-muted-foreground/20"
              onClick={() => removeEnvironmentFromConnection(connectionId, env)}
            >
              <X className="h-3 w-3" />
            </button>
          </Badge>
        ))}

      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm" className="h-6 gap-1 px-2 text-xs">
            <Plus className="h-3 w-3" />
            {environments.length === 0 ? "Add environment" : compact ? `${environments.length} env` : "Edit"}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-56 p-1" align="start">
          <p className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
            Environments
          </p>
          {availableEnvironments.length > 0 && (
            <div className="max-h-48 overflow-y-auto">
              {availableEnvironments.map((env) => (
                <button
                  key={env}
                  type="button"
                  className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-muted"
                  onClick={() => toggle(env)}
                >
                  <span
                    className={cn(
                      "flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center rounded-sm border",
                      assigned.has(env) ? "border-primary bg-primary text-primary-foreground" : "border-muted-foreground/40"
                    )}
                  >
                    {assigned.has(env) && <Check className="h-2.5 w-2.5" />}
                  </span>
                  <span className="flex-1 truncate">{env}</span>
                </button>
              ))}
            </div>
          )}
          <div className="mt-1 flex gap-1 border-t p-1">
            <Input
              value={newEnv}
              onChange={(e) => setNewEnv(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault()
                  addNew()
                }
              }}
              placeholder="New environment"
              className="h-7 text-xs"
            />
            <Button size="sm" className="h-7 px-2 text-xs" onClick={addNew} disabled={!newEnv.trim()}>
              Add
            </Button>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  )
}
