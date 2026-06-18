/**
 * Connection Export Service
 *
 * Handles exporting database connections to a downloadable JSON file.
 * See ADR-027 for architecture decisions.
 *
 * @module lib/export-import/export-service
 */

import { useConnectionStore, type DatabaseConnection } from '@/store/connection-store'
import { getSecureStorage } from '@/lib/secure-storage'
import { isWailsEnvironment } from '@/lib/wails-runtime'

import {
  ConnectionExportFile,
  CURRENT_SCHEMA_VERSION,
  ExportedConnection,
  ExportOptions,
} from './types'

// App version - would ideally come from package.json or env
const APP_VERSION = '1.0.0'

/**
 * Export connections to a JSON file. On the desktop build the file is written
 * natively to the Downloads folder (the path is returned); on the web build a
 * browser download is triggered. Returns the saved file path when known.
 */
export async function exportConnections(options: ExportOptions = { includePasswords: false }): Promise<string | void> {
  const exportFile = await buildExportFile(options)
  return downloadExportFile(exportFile)
}

/**
 * Build the export file structure from current connections
 */
export async function buildExportFile(options: ExportOptions): Promise<ConnectionExportFile> {
  const state = useConnectionStore.getState()
  let connections = state.connections

  // Filter to selected connections if specified
  if (options.selectedConnectionIds && options.selectedConnectionIds.length > 0) {
    const selectedSet = new Set(options.selectedConnectionIds)
    connections = connections.filter(c => selectedSet.has(c.id))
  }

  // Transform connections for export
  const exportedConnections = await Promise.all(
    connections.map(conn => transformConnectionForExport(conn, options.includePasswords))
  )

  return {
    metadata: {
      version: CURRENT_SCHEMA_VERSION,
      exportedAt: new Date().toISOString(),
      appVersion: APP_VERSION,
      connectionCount: exportedConnections.length,
      includesPasswords: options.includePasswords,
    },
    connections: exportedConnections,
  }
}

/**
 * Transform a single connection for export
 * Strips runtime-only fields and optionally includes password
 */
async function transformConnectionForExport(
  connection: DatabaseConnection,
  includePassword: boolean
): Promise<ExportedConnection> {
  const exported: ExportedConnection = {
    id: connection.id,
    name: connection.name,
    type: connection.type,
    host: connection.host,
    port: connection.port,
    database: connection.database,
    username: connection.username,
    sslMode: connection.sslMode,
    environments: connection.environments,
    useTunnel: connection.useTunnel,
    useVpc: connection.useVpc,
    parameters: sanitizeParameters(connection.parameters),
  }

  // SSH Tunnel config (without password/privateKey content)
  if (connection.sshTunnel) {
    exported.sshTunnel = {
      host: connection.sshTunnel.host,
      port: connection.sshTunnel.port,
      user: connection.sshTunnel.user,
      authMethod: connection.sshTunnel.authMethod,
      privateKeyPath: connection.sshTunnel.privateKeyPath,
      knownHostsPath: connection.sshTunnel.knownHostsPath,
      strictHostKeyChecking: connection.sshTunnel.strictHostKeyChecking,
      timeoutSeconds: connection.sshTunnel.timeoutSeconds,
      keepAliveIntervalSeconds: connection.sshTunnel.keepAliveIntervalSeconds,
    }
  }

  // VPC config (sanitized)
  if (connection.vpcConfig) {
    exported.vpcConfig = {
      vpcId: connection.vpcConfig.vpcId,
      subnetId: connection.vpcConfig.subnetId,
      securityGroupIds: connection.vpcConfig.securityGroupIds,
      privateLinkService: connection.vpcConfig.privateLinkService,
      endpointServiceName: connection.vpcConfig.endpointServiceName,
      // NOTE: customConfig is intentionally excluded
    }
  }

  // Optionally include database password
  if (includePassword) {
    try {
      const secureStorage = getSecureStorage()
      const credentials = await secureStorage.getCredentials(connection.id)
      if (credentials?.password) {
        exported.password = credentials.password
      }
    } catch {
      // Secure storage unavailable - skip password
    }
  }

  return exported
}

/**
 * Sanitize connection parameters - remove any that might be sensitive
 */
function sanitizeParameters(params?: Record<string, string>): Record<string, string> | undefined {
  if (!params) return undefined

  const sensitiveKeys = ['password', 'secret', 'key', 'token', 'credential']
  const sanitized: Record<string, string> = {}

  for (const [key, value] of Object.entries(params)) {
    const lowerKey = key.toLowerCase()
    const isSensitive = sensitiveKeys.some(sk => lowerKey.includes(sk))
    if (!isSensitive) {
      sanitized[key] = value
    }
  }

  return Object.keys(sanitized).length > 0 ? sanitized : undefined
}

/**
 * Persist the export file. In the Wails desktop webview the browser's
 * `<a download>` trick is a no-op (the embedded WebKit view ignores the
 * download attribute), so we write the file natively via the FileService —
 * the same mechanism the query-results export uses. On the web we fall back to
 * a real browser download. Returns the saved path when written natively.
 */
async function downloadExportFile(exportFile: ConnectionExportFile): Promise<string | void> {
  const json = JSON.stringify(exportFile, null, 2)
  const date = new Date().toISOString().split('T')[0]
  const filename = `howlerops-connections-${date}.json`

  if (isWailsEnvironment()) {
    const { SaveToDownloads } = await import('../../../bindings/github.com/jbeck018/howlerops/app')
    return await SaveToDownloads(filename, json)
  }

  const blob = new Blob([json], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

/**
 * Get the list of connections available for export
 */
export function getExportableConnections(): Array<{ id: string; name: string; type: string }> {
  const state = useConnectionStore.getState()
  return state.connections.map(c => ({
    id: c.id,
    name: c.name,
    type: c.type,
  }))
}
