import { Bell, BookOpen, Database, FileText, GitCompare, Notebook, Settings, Terminal } from "lucide-react"
import { Link, useLocation, useNavigate } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

// Navigation items configuration (single source of truth for both layouts)
export const NAV_ITEMS = [
  { path: '/dashboard', label: 'Queries', icon: Terminal },
  { path: '/connections', label: 'Connections', icon: Database },
  { path: '/reports', label: 'Reports', icon: FileText },
  { path: '/notebooks', label: 'Notebooks', icon: Notebook },
  { path: '/alerts', label: 'Alerts', icon: Bell },
  { path: '/schema-diff', label: 'Schema Diff', icon: GitCompare },
  { path: '/data-catalog', label: 'Data Catalog', icon: BookOpen },
  { path: '/settings', label: 'Settings', icon: Settings },
] as const

interface SidebarNavProps {
  collapsed?: boolean
}

/**
 * Renders the primary navigation. The collapsed layout shows icon-only buttons;
 * the expanded layout shows labelled links. Both derive from NAV_ITEMS so the
 * list only needs to be maintained in one place.
 */
export function SidebarNav({ collapsed = false }: SidebarNavProps) {
  const location = useLocation()
  const navigate = useNavigate()

  if (collapsed) {
    return (
      <>
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon
          const isActive = location.pathname === item.path
          return (
            <Button
              key={item.path}
              variant={isActive ? "secondary" : "ghost"}
              size="icon"
              className="h-8 w-8 p-0"
              onClick={() => navigate(item.path)}
              title={item.label}
            >
              <Icon className="h-4 w-4" />
            </Button>
          )
        })}
      </>
    )
  }

  return (
    <nav className="px-3 space-y-1">
      {NAV_ITEMS.map((item) => {
        const Icon = item.icon
        const isActive = location.pathname === item.path
        return (
          <Link
            key={item.path}
            to={item.path}
            className={cn(
              "flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors",
              isActive
                ? "bg-secondary text-secondary-foreground"
                : "text-muted-foreground hover:text-foreground hover:bg-muted"
            )}
          >
            <Icon className="h-4 w-4 flex-shrink-0" />
            <span className="truncate">{item.label}</span>
          </Link>
        )
      })}
    </nav>
  )
}
