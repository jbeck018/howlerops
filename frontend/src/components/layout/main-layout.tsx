import { InvitationBanner } from "@/components/InvitationBanner"
import { useLayoutStore } from "@/store/layout-store"

import { ContextPanel } from "./context-panel"
import { Header } from "./header"
import { IconRail } from "./icon-rail"

interface MainLayoutProps {
  children: React.ReactNode
}

export function MainLayout({ children }: MainLayoutProps) {
  const contextPanelCollapsed = useLayoutStore((state) => state.contextPanelCollapsed)

  return (
    <div className="flex h-screen flex-col">
      <Header />
      <InvitationBanner />
      <div className="flex flex-1 min-h-0 overflow-hidden relative">
        <IconRail />
        {!contextPanelCollapsed && <ContextPanel />}
        <main className="flex-1 bg-bg relative flex min-h-0 overflow-hidden">
          <div className="flex-1 flex min-h-0 flex-col overflow-hidden">
            {children}
          </div>
        </main>
      </div>
    </div>
  )
}
