import { Plus, Sparkles, Terminal, X } from "lucide-react"
import { useShallow } from "zustand/react/shallow"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { useQueryEditorStore } from "@/store/query-editor-store"

/**
 * Vertical "Open Tabs" list for the Queries sub-nav — the replacement for the
 * horizontal Query/AI tab strip. Reads the query-editor store directly (it lives
 * in the ContextPanel, a sibling of the editor) and drives tab switching via
 * setActiveTab, which the editor already syncs to reactively.
 */
export function OpenTabsList() {
  const { tabs, activeTabId, setActiveTab, closeTab, createTab } = useQueryEditorStore(
    useShallow((state) => ({
      tabs: state.tabs,
      activeTabId: state.activeTabId,
      setActiveTab: state.setActiveTab,
      closeTab: state.closeTab,
      createTab: state.createTab,
    }))
  )

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
            <DropdownMenuItem onClick={() => createTab("New Query", { type: "sql" })}>
              <Terminal className="h-4 w-4 mr-2" />
              SQL Editor
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
              return (
                <div
                  key={tab.id}
                  className={cn(
                    "group relative flex items-center gap-2 rounded-md pl-2 pr-1 py-1.5 cursor-pointer transition-colors",
                    isActive ? "bg-muted" : "hover:bg-muted/50"
                  )}
                  onClick={() => setActiveTab(tab.id)}
                >
                  {isActive && (
                    <span className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-primary" />
                  )}
                  {tab.type === "ai" ? (
                    <Sparkles className="h-3.5 w-3.5 flex-shrink-0 text-accent-foreground" />
                  ) : (
                    <Terminal className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
                  )}
                  <span className="flex-1 min-w-0 truncate text-xs">{tab.title}</span>
                  {tab.isDirty && <span className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary" />}
                  {tabs.length > 1 && (
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
                  )}
                </div>
              )
            })
          )}
        </div>
      </ScrollArea>
    </div>
  )
}
