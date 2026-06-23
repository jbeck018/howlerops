import { Moon, Sun } from "lucide-react"
import { Link } from "react-router-dom"

import { TierBadge } from "@/components/tier-badge"
import { HowlerOpsIcon } from "@/components/ui/howlerops-icon"
import { Switch } from "@/components/ui/switch"
import { useTheme } from "@/hooks/use-theme"

export function Header() {
  const { theme, setTheme } = useTheme()

  const toggleTheme = () => {
    setTheme(theme === "light" ? "dark" : "light")
  }

  return (
    <header className="sticky top-0 z-50 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      {/* Native title bar drag zone — sits behind macOS traffic lights */}
      <div className="h-11" />
      {/* Header content — below the native title bar */}
      <div className="flex h-10 items-center border-b px-4 pb-2">
        {/* Logo */}
        <Link to="/dashboard" className="flex items-center space-x-2">
          <HowlerOpsIcon size={24} variant={theme === "dark" ? "dark" : "light"} />
          <h1 className="text-lg font-semibold">HowlerOps</h1>
        </Link>

        {/* Right side actions */}
        <div className="ml-auto flex items-center space-x-3">
          {/*
            Sign In and the New Query menu are hidden for now. They belong to the
            syncable / hosted deployment story (auth + cloud sync) which isn't
            ready yet. New query tabs can still be created from the Queries
            sub-nav "Open Tabs" list. Restore <AuthButton /> and the New Query
            dropdown here once those flows return.
          */}
          <TierBadge variant="header" />

          <div className="flex items-center space-x-2 border-l pl-3 ml-1">
            <Sun className="h-4 w-4 text-muted-foreground" />
            <Switch
              checked={theme === "dark"}
              onCheckedChange={toggleTheme}
              aria-label="Toggle theme"
            />
            <Moon className="h-4 w-4 text-muted-foreground" />
          </div>
        </div>
      </div>
    </header>
  )
}
