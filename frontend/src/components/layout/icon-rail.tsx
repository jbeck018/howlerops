import { useLocation, useNavigate } from "react-router-dom"

import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

import { NAV_ITEMS } from "./sidebar-nav"

/**
 * VS Code-style 48px vertical icon rail. Renders NAV_ITEMS as icon-only
 * buttons with tooltips. Settings is pinned to the bottom.
 */
export function IconRail() {
  const location = useLocation()
  const navigate = useNavigate()

  return (
    <TooltipProvider delayDuration={200}>
      <div className="w-12 flex-shrink-0 border-r bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 flex flex-col items-stretch py-2">
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon
          const isActive = location.pathname === item.path
          const isSettings = item.path === '/settings'

          return (
            <Tooltip key={item.path}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={item.label}
                  aria-current={isActive ? "page" : undefined}
                  onClick={() => navigate(item.path)}
                  className={cn(
                    "relative flex h-11 items-center justify-center transition-colors",
                    isSettings && "mt-auto",
                    isActive
                      ? "text-foreground bg-muted"
                      : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                  )}
                >
                  {isActive && (
                    <span className="absolute left-0 top-1/2 h-6 w-0.5 -translate-y-1/2 rounded-full bg-primary" />
                  )}
                  <Icon className="h-5 w-5" />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right">{item.label}</TooltipContent>
            </Tooltip>
          )
        })}
      </div>
    </TooltipProvider>
  )
}
