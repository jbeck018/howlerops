import { ArrowRight, Loader2, RotateCw } from "lucide-react"

import { useUpdateChecker } from "@/hooks/use-update-checker"
import { cn } from "@/lib/utils"

/**
 * Pinned sidebar card that appears when the background update poller finds a
 * newer release. Clicking it installs the update in place and relaunches the
 * app (the one-click "Relaunch to update" flow). It renders nothing until an
 * update is actually available, so it stays out of the way otherwise.
 */
export function SidebarUpdateCard() {
  const { updateInfo, isInstalling, error, downloadAndRestart } = useUpdateChecker()

  if (!updateInfo?.available) {
    return null
  }

  const version = updateInfo.latestVersion?.startsWith("v")
    ? updateInfo.latestVersion
    : `v${updateInfo.latestVersion}`

  return (
    <div className="border-t p-2">
      <button
        type="button"
        onClick={downloadAndRestart}
        disabled={isInstalling}
        className={cn(
          "group flex w-full items-center gap-3 rounded-lg border bg-card px-3 py-2.5 text-left",
          "shadow-sm transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-70"
        )}
        title={`Update to ${version} and relaunch`}
      >
        <span className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          {isInstalling ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <RotateCw className="h-4 w-4" />
          )}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold">
            {isInstalling ? "Updating…" : "Relaunch to update"}
          </span>
          <span className="block truncate text-xs text-muted-foreground">{version}</span>
        </span>
        {!isInstalling && (
          <ArrowRight className="h-4 w-4 flex-shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
        )}
      </button>
      {error && <p className="mt-1.5 px-1 text-xs text-destructive">{error}</p>}
    </div>
  )
}
