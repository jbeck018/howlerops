import { Check, Plus, Sparkles, Terminal, X } from "lucide-react"
import { useRef, useState } from "react"
import { useShallow } from "zustand/react/shallow"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { useQueryEditorStore } from "@/store/query-editor-store"

import { useTabActions } from "./use-tab-actions"

/**
 * Vertical "Open Tabs" list for the Queries sub-nav — the replacement for the
 * horizontal Query/AI tab strip. Reads the query-editor store directly and drives
 * tab switching via setActiveTab (the editor syncs content reactively). Tab
 * creation and per-tab connection changes go through useTabActions, which is
 * shared with the editor so behaviour stays consistent.
 */
export function OpenTabsList() {
  const { tabs, activeTabId, setActiveTab, closeTab, updateTab } = useQueryEditorStore(
    useShallow((state) => ({
      tabs: state.tabs,
      activeTabId: state.activeTabId,
      setActiveTab: state.setActiveTab,
      closeTab: state.closeTab,
      updateTab: state.updateTab,
    }))
  )
  const { connections, createSqlTab, createAiTab, changeTabConnection, getConnectionLabelForTab } =
    useTabActions()

  const [openConnPopover, setOpenConnPopover] = useState<string | null>(null)

  // Inline tab rename. escapedRef guards against the Escape-then-blur race so
  // cancelling doesn't commit the in-progress draft.
  const [editingId, setEditingId] = useState<string | null>(null)
  const [draftTitle, setDraftTitle] = useState("")
  const escapedRef = useRef(false)

  const startRename = (id: string, title: string) => {
    escapedRef.current = false
    setEditingId(id)
    setDraftTitle(title)
  }

  const commitRename = (id: string) => {
    if (escapedRef.current) {
      escapedRef.current = false
      setEditingId(null)
      return
    }
    const name = draftTitle.trim()
    if (name) updateTab(id, { title: name })
    setEditingId(null)
  }

  return (
    <div className="flex w-56 flex-shrink-0 flex-col border-b border-r max-h-[45%] bg-background/95">
      <div className="flex items-center justify-between px-3 pt-3 pb-1">
        <div className="flex items-center gap-2 text-muted-foreground">
          <Terminal className="h-4 w-4" />
          <span className="text-[11px] font-semibold uppercase tracking-wide">Open Tabs</span>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className="h-6 w-6 p-0" title="New tab">
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => createSqlTab()}>
              <Terminal className="h-4 w-4 mr-2" />
              SQL Editor
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => createAiTab()}>
              <Sparkles className="h-4 w-4 mr-2" />
              AI Query Agent
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <ScrollArea className="flex-1">
        <div className="space-y-0.5 px-2 pb-2">
          {tabs.length === 0 ? (
            <p className="px-2 py-2 text-[11px] text-muted-foreground">No open tabs.</p>
          ) : (
            tabs.map((tab) => {
              const isActive = tab.id === activeTabId
              const connLabel = getConnectionLabelForTab(tab)
              return (
                <div
                  key={tab.id}
                  className={cn(
                    "group relative flex items-center gap-2 rounded-md pl-2 pr-1 py-1.5 cursor-pointer transition-colors",
                    isActive ? "bg-muted" : "hover:bg-muted/50"
                  )}
                  onClick={() => setActiveTab(tab.id)}
                  title={`${tab.title} · ${connLabel}`}
                >
                  {isActive && (
                    <span className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-primary" />
                  )}
                  {tab.type === "ai" ? (
                    <Sparkles className="h-3.5 w-3.5 flex-shrink-0 text-accent-foreground" />
                  ) : (
                    <Terminal className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
                  )}
                  <div className="flex flex-1 min-w-0 flex-col">
                    {editingId === tab.id ? (
                      <input
                        autoFocus
                        value={draftTitle}
                        onChange={(e) => setDraftTitle(e.target.value)}
                        onClick={(e) => e.stopPropagation()}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            e.preventDefault()
                            commitRename(tab.id)
                          } else if (e.key === "Escape") {
                            e.preventDefault()
                            escapedRef.current = true
                            setEditingId(null)
                          }
                        }}
                        onBlur={() => commitRename(tab.id)}
                        className="w-full rounded border bg-background px-1 text-xs leading-tight outline-none focus:ring-1 focus:ring-primary"
                      />
                    ) : (
                      <span
                        className="truncate text-xs leading-tight"
                        onDoubleClick={(e) => {
                          e.stopPropagation()
                          startRename(tab.id, tab.title)
                        }}
                        title="Double-click to rename"
                      >
                        {tab.title}
                      </span>
                    )}
                    <Popover
                      open={openConnPopover === tab.id}
                      onOpenChange={(open) => setOpenConnPopover(open ? tab.id : null)}
                    >
                      <PopoverTrigger asChild>
                        <button
                          type="button"
                          className="truncate text-left text-[10px] text-muted-foreground hover:text-foreground"
                          onClick={(e) => e.stopPropagation()}
                        >
                          {connLabel}
                        </button>
                      </PopoverTrigger>
                      <PopoverContent className="w-56 p-1" align="start" onClick={(e) => e.stopPropagation()}>
                        <p className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                          Connection
                        </p>
                        <div className="max-h-60 overflow-y-auto">
                          {connections.length === 0 ? (
                            <p className="px-2 py-1.5 text-xs text-muted-foreground">No connections.</p>
                          ) : (
                            connections.map((conn) => (
                              <button
                                key={conn.id}
                                type="button"
                                className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-muted"
                                onClick={() => {
                                  void changeTabConnection(tab.id, conn.id)
                                  setOpenConnPopover(null)
                                }}
                              >
                                <span
                                  className={cn(
                                    "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                                    conn.isConnected ? "bg-green-500" : "bg-muted-foreground/40"
                                  )}
                                />
                                <span className="flex-1 truncate">{conn.name}</span>
                                {tab.connectionId === conn.id && <Check className="h-3 w-3 flex-shrink-0" />}
                              </button>
                            ))
                          )}
                        </div>
                      </PopoverContent>
                    </Popover>
                  </div>
                  {tab.isDirty && <span className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary" />}
                  {/* Always allow closing — closing the last tab drops to the
                      editor's empty state (the store supports zero tabs). */}
                  <button
                    type="button"
                    aria-label="Close tab"
                    className="flex-shrink-0 rounded p-0.5 opacity-0 group-hover:opacity-100 hover:bg-muted-foreground/20"
                    onClick={(e) => {
                      e.stopPropagation()
                      closeTab(tab.id)
                    }}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              )
            })
          )}
        </div>
      </ScrollArea>
    </div>
  )
}
