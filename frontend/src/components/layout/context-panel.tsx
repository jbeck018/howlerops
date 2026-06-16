import { BookOpen, FileText, GitCompare, Settings as SettingsIcon } from "lucide-react"
import { Link, useLocation } from "react-router-dom"

import { ScrollArea } from "@/components/ui/scroll-area"

import { Sidebar } from "./sidebar"
import { OpenTabsList } from "./subnav/open-tabs-list"

/**
 * Route-aware contextual panel (column 2 of the IconRail + ContextPanel + content
 * layout). It swaps its body based on the active route so each primary
 * destination gets its own sub-nav. The Queries and Connections routes reuse the
 * connections-focused Sidebar; other routes get a lean labelled panel that links
 * into the page. These are intentionally minimal scaffolds to be fleshed out.
 */
export function ContextPanel() {
  const { pathname } = useLocation()

  // Connections context is most useful where you work against databases.
  if (pathname.startsWith("/dashboard")) {
    // Queries route: open-tabs list stacked above the connections panel.
    return (
      <div className="flex h-full min-h-0 w-56 flex-shrink-0 flex-col border-r bg-background/95">
        <OpenTabsList />
        <div className="flex-1 min-h-0">
          <Sidebar />
        </div>
      </div>
    )
  }

  if (pathname.startsWith("/connections")) {
    return <Sidebar />
  }

  if (pathname.startsWith("/reports")) {
    return (
      <SubNavShell title="Reports" icon={<FileText className="h-4 w-4" />}>
        <SubNavLink to="/reports" label="All reports" />
        <SubNavHint>Build and schedule report dashboards.</SubNavHint>
      </SubNavShell>
    )
  }

  if (pathname.startsWith("/schema-diff")) {
    return (
      <SubNavShell title="Schema Diff" icon={<GitCompare className="h-4 w-4" />}>
        <SubNavLink to="/schema-diff" label="Compare schemas" />
        <SubNavHint>Diff two connections or snapshots.</SubNavHint>
      </SubNavShell>
    )
  }

  if (pathname.startsWith("/data-catalog")) {
    return (
      <SubNavShell title="Data Catalog" icon={<BookOpen className="h-4 w-4" />}>
        <SubNavLink to="/data-catalog" label="Browse catalog" />
        <SubNavHint>Search tables, columns and relationships.</SubNavHint>
      </SubNavShell>
    )
  }

  if (pathname.startsWith("/settings")) {
    return (
      <SubNavShell title="Settings" icon={<SettingsIcon className="h-4 w-4" />}>
        <SubNavLink to="/settings" label="All settings" />
        <SubNavHint>Connections, AI providers, appearance and more.</SubNavHint>
      </SubNavShell>
    )
  }

  // Default: show the connections panel.
  return <Sidebar />
}

function SubNavShell({
  title,
  icon,
  children,
}: {
  title: string
  icon: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="w-56 border-r bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 flex-shrink-0 flex flex-col">
      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-1 p-2 pt-3">
          <div className="flex items-center gap-2 px-2 pb-1 text-muted-foreground">
            {icon}
            <span className="text-[11px] font-semibold uppercase tracking-wide">{title}</span>
          </div>
          {children}
        </div>
      </ScrollArea>
    </div>
  )
}

function SubNavLink({ to, label }: { to: string; label: string }) {
  return (
    <Link
      to={to}
      className="rounded-md px-2 py-1.5 text-xs text-foreground hover:bg-muted transition-colors"
    >
      {label}
    </Link>
  )
}

function SubNavHint({ children }: { children: React.ReactNode }) {
  return <p className="px-2 pt-1 text-[11px] leading-snug text-muted-foreground">{children}</p>
}
