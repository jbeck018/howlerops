import type { ColumnInfo, SchemaInfo, TableInfo } from "@/components/visual-query-builder/types"
import { type SchemaNode } from "@/hooks/use-schema-introspection"

/**
 * Map connection database type to SQL dialect for query generation
 */
export type SqlDialect = 'postgres' | 'mysql' | 'sqlite' | 'mssql'

export function getDialectFromConnectionType(connectionType: string | undefined): SqlDialect {
  if (!connectionType) return 'postgres'
  switch (connectionType.toLowerCase()) {
    case 'postgresql':
    case 'postgres':
      return 'postgres'
    case 'mysql':
    case 'mariadb':
    case 'tidb':
      return 'mysql'
    case 'sqlite':
      return 'sqlite'
    case 'mssql':
    case 'sqlserver':
      return 'mssql'
    default:
      return 'postgres'
  }
}

/**
 * Convert SchemaNode[] (hierarchical tree) to SchemaInfo[] (flat list for visual query builder)
 */
export function convertSchemaNodes(schemaNodes: SchemaNode[]): SchemaInfo[] {
  const result: SchemaInfo[] = []

  for (const schemaOrDb of schemaNodes) {
    // Handle both database-level nodes and schema-level nodes
    if (schemaOrDb.type === 'schema') {
      const tables: TableInfo[] = []
      for (const tableNode of schemaOrDb.children || []) {
        if (tableNode.type === 'table') {
          const columns: ColumnInfo[] = []
          for (const colNode of tableNode.children || []) {
            if (colNode.type === 'column') {
              const meta = colNode.metadata as Record<string, unknown> | undefined
              columns.push({
                name: colNode.name,
                dataType: (meta?.dataType as string) || (meta?.data_type as string) || 'unknown',
                isNullable: meta?.isNullable === true || meta?.nullable === true || meta?.isNullable === 'YES',
                isPrimaryKey: meta?.isPrimaryKey === true || meta?.primaryKey === true,
                isForeignKey: meta?.isForeignKey === true || meta?.foreignKey === true,
              })
            }
          }
          const tableMeta = tableNode.metadata as Record<string, unknown> | undefined
          tables.push({
            name: tableNode.name,
            schema: schemaOrDb.name,
            columns,
            rowCount: tableMeta?.rowCount as number | undefined,
            sizeBytes: tableMeta?.sizeBytes as number | undefined,
          })
        }
      }
      result.push({ name: schemaOrDb.name, tables })
    } else if (schemaOrDb.type === 'database') {
      // Recurse into database children (which should be schemas)
      for (const childSchema of schemaOrDb.children || []) {
        if (childSchema.type === 'schema') {
          result.push(...convertSchemaNodes([childSchema]))
        }
      }
    }
  }

  return result
}
