import { create } from "zustand"
import { persist } from "zustand/middleware"

/**
 * UI layout state that should survive reloads — currently whether the
 * route-aware context panel (column 2) is collapsed down to just the icon rail.
 */
interface LayoutState {
  contextPanelCollapsed: boolean
  toggleContextPanel: () => void
  setContextPanelCollapsed: (collapsed: boolean) => void
}

export const useLayoutStore = create<LayoutState>()(
  persist(
    (set) => ({
      contextPanelCollapsed: false,
      toggleContextPanel: () =>
        set((state) => ({ contextPanelCollapsed: !state.contextPanelCollapsed })),
      setContextPanelCollapsed: (collapsed) => set({ contextPanelCollapsed: collapsed }),
    }),
    { name: "layout-store" }
  )
)
