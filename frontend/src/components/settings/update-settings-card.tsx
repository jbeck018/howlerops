import { Check, Copy, Download, ExternalLink, RefreshCw, Terminal } from "lucide-react"
import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useToast } from "@/hooks/use-toast"
import type { UpdateInfo } from "@/hooks/use-update-checker"
import { isWailsEnvironment } from "@/lib/wails-runtime"

import { CheckForUpdates, GetCurrentVersion, OpenDownloadPage } from "../../../bindings/github.com/jbeck018/howlerops/app"

// The curl one-liner is the supported way to install/update HowlerOps. It
// resolves the latest release and replaces the installed binary/app in place.
const INSTALL_COMMAND = "curl -fsSL https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh | sh"

/**
 * Settings card for application updates. On the desktop (Wails) build it shows
 * the running version and lets the user check GitHub for a newer release on
 * demand — distinct from the passive UpdateNotification toast, this check is
 * authoritative and ignores any previously "dismissed" version. Everywhere it
 * surfaces the curl installer command, which is how updates are actually
 * applied (there is no in-app binary self-update).
 */
export function UpdateSettingsCard() {
  const { toast } = useToast()
  const isDesktop = isWailsEnvironment()

  const [currentVersion, setCurrentVersion] = useState<string>("")
  const [isChecking, setIsChecking] = useState(false)
  const [result, setResult] = useState<UpdateInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!isDesktop) return
    GetCurrentVersion()
      .then((v) => setCurrentVersion(v))
      .catch(() => setCurrentVersion(""))
  }, [isDesktop])

  const handleCheck = async () => {
    if (!isDesktop) return
    setIsChecking(true)
    setError(null)
    try {
      const info = await CheckForUpdates()
      setResult((info ?? null) as UpdateInfo | null)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to check for updates")
      setResult(null)
    } finally {
      setIsChecking(false)
    }
  }

  const handleOpenReleasePage = async () => {
    try {
      await OpenDownloadPage()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to open release page")
    }
  }

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(INSTALL_COMMAND)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
      toast({ title: "Copied", description: "Install command copied to clipboard." })
    } catch {
      toast({
        title: "Couldn't copy",
        description: "Copy the command manually.",
        variant: "destructive",
      })
    }
  }

  const displayVersion = currentVersion || "dev"

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Download className="h-5 w-5" />
          Updates
        </CardTitle>
        <CardDescription>Check for new versions of HowlerOps and keep your install current.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Current version + check button */}
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium">Current version</p>
            <p className="text-sm text-muted-foreground">{isDesktop ? displayVersion : "—"}</p>
          </div>
          {isDesktop && (
            <Button variant="outline" size="sm" onClick={handleCheck} disabled={isChecking} className="gap-2">
              <RefreshCw className={`h-4 w-4 ${isChecking ? "animate-spin" : ""}`} />
              {isChecking ? "Checking…" : "Check for updates"}
            </Button>
          )}
        </div>

        {/* Status of the most recent check */}
        {isDesktop && error && <p className="text-sm text-destructive">{error}</p>}

        {isDesktop && !error && result?.available && (
          <div className="flex items-center justify-between gap-4 rounded-lg border border-primary/20 bg-primary/5 p-3">
            <div>
              <p className="text-sm font-medium">Update available</p>
              <p className="text-sm text-muted-foreground">
                Version {result.latestVersion} is available. Update with the command below or open the release page.
              </p>
            </div>
            <Button size="sm" variant="outline" onClick={handleOpenReleasePage} className="gap-2 whitespace-nowrap">
              <ExternalLink className="h-4 w-4" />
              Release page
            </Button>
          </div>
        )}

        {isDesktop && !error && result && !result.available && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Check className="h-4 w-4 text-green-500" />
            You&apos;re on the latest version.
          </p>
        )}

        {/* Install / update command (always shown) */}
        <div className="space-y-2">
          <p className="flex items-center gap-2 text-sm font-medium">
            <Terminal className="h-4 w-4" />
            Install or update from your terminal
          </p>
          <div className="flex items-center gap-2 rounded-md border bg-muted/40 p-2">
            <code className="flex-1 overflow-x-auto whitespace-nowrap px-1 text-xs">{INSTALL_COMMAND}</code>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 flex-shrink-0"
              onClick={handleCopy}
              aria-label="Copy install command"
            >
              {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
            </Button>
          </div>
          {!isDesktop && (
            <p className="text-xs text-muted-foreground">
              In-app update checks are available in the desktop app. On the web, use the command above.
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
