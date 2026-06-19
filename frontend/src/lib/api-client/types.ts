/**
 * Unified API Client Types
 *
 * These types define the interface for both Wails desktop and REST web clients.
 * Both implementations must conform to these interfaces.
 */

// ==========================================
// Common Response Types
// ==========================================

export interface ApiResponse<T> {
  data: T
  success: boolean
  message?: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
}

// ==========================================
// Connection Types
// ==========================================

export interface ConnectionInfo {
  id: string
  name: string
  description?: string
  type: string
  host: string
  port: number
  database: string
  username: string
  active: boolean
  createdAt: string
  updatedAt: string
  createdBy?: string
  tags?: Record<string, string>
}

export interface CreateConnectionRequest {
  id?: string
  name?: string
  type?: string
  host?: string
  port?: number
  database?: string
  username?: string
  password?: string
  ssl_mode?: string
  connection_timeout?: number
  parameters?: Record<string, string>
}

export interface SaveConnectionRequest {
  id?: string
  name?: string
  type?: string
  host?: string
  port?: number
  database?: string
  username?: string
  password?: string
  ssl_mode?: string
  connection_timeout?: number
  parameters?: Record<string, string>
}

export interface TestConnectionRequest {
  type?: string
  host?: string
  port?: number
  database?: string
  username?: string
  password?: string
  ssl_mode?: string
  connection_timeout?: number
  parameters?: Record<string, string>
}

export interface TestConnectionResult {
  success: boolean
  responseTime: number
  version: string
  serverInfo: Record<string, unknown>
}

export interface ListDatabasesResult {
  success: boolean
  message?: string
  databases: string[]
}

export interface SwitchDatabaseResult {
  success: boolean
  message?: string
  database: string
  reconnected: boolean
}

// ==========================================
// Query Types
// ==========================================

/**
 * Optional per-execution targeting options.
 * - `database`: run against a specific database on the connection without
 *   globally switching the connection's active database (per-tab targeting).
 * - `selectedConnectionIds`: scope multi-DB (@conn) federation to these
 *   connections only. Empty/undefined = all connected connections.
 */
export interface QueryExecOptions {
  database?: string
  selectedConnectionIds?: string[]
}

export interface QueryResult {
  queryId: string
  success: boolean
  columns: QueryColumn[]
  rows: unknown[][]
  rowCount: number
  stats: QueryStats
  warnings: string[]
  editable?: EditableMetadata | null
  totalRows?: number
  pagedRows?: number
  hasMore?: boolean
  offset?: number
  connectionsUsed?: string[]
}

export interface QueryColumn {
  name: string
  dataType: string
  nullable?: boolean
}

export interface QueryStats {
  duration?: string | number  // Backend may return "123ms" string or numeric ms
  affectedRows?: number
  affected_rows?: number  // Snake case variant
}

export interface EditableColumn {
  name: string
  dataType: string
  nullable?: boolean
  primaryKey?: boolean
}

export interface EditableMetadata {
  enabled?: boolean
  reason?: string
  schema?: string
  table?: string
  primaryKey?: string[]
  primaryKeys?: string[]  // Variant naming
  columns?: string[] | EditableColumn[]
  pending?: boolean
  jobId?: string
  job_id?: string  // Snake case variant
  capabilities?: {
    canInsert?: boolean
    canUpdate?: boolean
    canDelete?: boolean
    reason?: string
  }
}

export interface UpdateRowRequest {
  connectionId: string
  query: string
  columns: string[]
  schema?: string
  table?: string
  primaryKey: Record<string, unknown>
  values: Record<string, unknown>
}

export interface InsertRowRequest {
  connectionId: string
  query: string
  columns: string[]
  schema?: string
  table?: string
  values: Record<string, unknown>
}

export interface DeleteRowsRequest {
  connectionId: string
  query: string
  columns: string[]
  schema?: string
  table?: string
  primaryKeys: Record<string, unknown>[]
}

export interface InsertRowResult {
  success: boolean
  message?: string
  row?: unknown[]
}

export interface DeleteRowsResult {
  success: boolean
  message?: string
  deleted?: number
}

export interface ExplainResult {
  plan: string
  format: string
  estimatedStats: Record<string, unknown>
  warnings: string[]
}

export interface CancelResult {
  success: boolean
  message?: string
  wasRunning: boolean
}

// ==========================================
// Schema Types
// ==========================================

export interface SchemaInfo {
  name: string
  owner: string
  createdAt: string
  tableCount: number
  sizeBytes: number
  metadata: Record<string, unknown>
}

export interface TableInfo {
  name: string
  schema: string
  type?: string
  comment?: string
  createdAt?: string
  updatedAt?: string
  rowCount?: number
  sizeBytes?: number
  owner?: string
  metadata?: Record<string, unknown>
}

export interface ColumnInfo {
  name: string
  dataType: string
  nullable: boolean
  defaultValue?: string | null
  primaryKey: boolean
  unique?: boolean
  indexed?: boolean
  comment?: string
  ordinalPosition?: number
  characterMaximumLength?: number | null
  numericPrecision?: number | null
  numericScale?: number | null
  metadata?: Record<string, unknown>
}

export interface TableStructureResult {
  data: ColumnInfo[]
  table: TableInfo | null
  indexes: unknown[]
  foreignKeys: unknown[]
  triggers: unknown[]
  statistics: Record<string, unknown>
  success: boolean
  message?: string
}

/**
 * Raw payload of the batched schema endpoint (`GetConnectionSchemaFull`). Fields
 * mirror the Go `FullDatabaseSchema` DTO json tags; columns/foreign_keys stay in
 * the backend's snake_case shape, which the schema-store normalizers accept.
 */
export interface RawFullSchema {
  schemas: string[]
  tables: Array<Record<string, unknown>>
  structures: Array<{
    table: {
      schema: string
      name: string
      rowCount?: number
      sizeBytes?: number
      comment?: string
      [key: string]: unknown
    }
    columns: Array<Record<string, unknown>>
    foreign_keys?: unknown[]
    [key: string]: unknown
  }>
}

export interface FullSchemaResult {
  data: RawFullSchema | null
  success: boolean
  message?: string
}

// ==========================================
// API Client Interface
// ==========================================

/**
 * Unified API Client Interface
 *
 * Both WailsApiClient and RestApiClient must implement this interface.
 * This allows seamless switching between desktop and web modes.
 */
export interface ApiClient {
  connections: {
    list: (page?: number, pageSize?: number, filter?: string) => Promise<PaginatedResponse<ConnectionInfo>>
    create: (data: CreateConnectionRequest) => Promise<ApiResponse<unknown>>
    save: (data: SaveConnectionRequest) => Promise<ApiResponse<null>>
    test: (data: TestConnectionRequest) => Promise<ApiResponse<TestConnectionResult>>
    remove: (connectionId: string) => Promise<ApiResponse<null>>
    listDatabases: (connectionId: string) => Promise<ListDatabasesResult>
    switchDatabase: (connectionId: string, database: string) => Promise<SwitchDatabaseResult>
  }

  queries: {
    execute: (connectionId: string, sql: string, limit?: number, offset?: number, timeout?: number, isExport?: boolean, execOptions?: QueryExecOptions) => Promise<ApiResponse<QueryResult>>
    getEditableMetadata: (jobId: string) => Promise<ApiResponse<EditableMetadata | null>>
    updateRow: (payload: UpdateRowRequest) => Promise<ApiResponse<null>>
    insertRow: (payload: InsertRowRequest) => Promise<InsertRowResult>
    deleteRows: (payload: DeleteRowsRequest) => Promise<DeleteRowsResult>
    explain: (connectionId: string, query: string) => Promise<ApiResponse<ExplainResult>>
    cancel: (streamId: string) => Promise<CancelResult>
  }

  schema: {
    databases: (connectionId: string) => Promise<ApiResponse<SchemaInfo[]>>
    tables: (connectionId: string, schemaName?: string) => Promise<ApiResponse<TableInfo[]>>
    columns: (connectionId: string, schemaName: string, tableName: string) => Promise<TableStructureResult>
    /**
     * Batched introspection: schemas + tables + per-table structure in one call.
     * Optional — only the Wails (desktop) client implements it; web/REST mode
     * falls back to the databases -> tables -> columns path.
     */
    full?: (connectionId: string) => Promise<FullSchemaResult>
  }
}

/**
 * Platform type for runtime detection
 */
export type PlatformType = 'wails' | 'web'
