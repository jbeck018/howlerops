import {
  ChevronDown,
  ChevronRight,
  Columns,
  Folder,
  FolderOpen,
  Key,
  Table,
} from "lucide-react"
import { useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import type { SchemaNode } from "@/hooks/use-schema-introspection"

interface SchemaTreeProps {
  nodes: SchemaNode[]
  level?: number
}

function getIcon(node: SchemaNode, isExpanded: boolean) {
  switch (node.type) {
    case 'database':
    case 'schema':
      return isExpanded ? <FolderOpen className="h-4 w-4" /> : <Folder className="h-4 w-4" />
    case 'table':
      return <Table className="h-4 w-4" />
    case 'column':
      return node.name.includes('PK') ? <Key className="h-4 w-4" /> : <Columns className="h-4 w-4" />
    default:
      return <div className="h-4 w-4" />
  }
}

export function SchemaTree({ nodes, level = 0 }: SchemaTreeProps) {
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(
    () => new Set(nodes.filter(node => node.expanded).map(node => node.id))
  )

  const toggleNode = (nodeId: string) => {
    setExpandedNodes(prev => {
      const newSet = new Set(prev)
      if (newSet.has(nodeId)) {
        newSet.delete(nodeId)
      } else {
        newSet.add(nodeId)
      }
      return newSet
    })
  }

  return (
    <div className="space-y-1">
      {nodes.map((node) => {
        const isExpanded = expandedNodes.has(node.id)
        const hasChildren = Boolean(node.children && node.children.length > 0)

        return (
          <div key={node.id}>
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-start h-8 px-2"
              style={{ paddingLeft: `${8 + level * 16}px` }}
              onClick={() => {
                if (hasChildren) {
                  toggleNode(node.id)
                }
              }}
            >
              {hasChildren ? (
                <div className="mr-1">
                  {isExpanded ? (
                    <ChevronDown className="h-3 w-3" />
                  ) : (
                    <ChevronRight className="h-3 w-3" />
                  )}
                </div>
              ) : (
                <div className="w-4" />
              )}
              <div className="mr-2">{getIcon(node, isExpanded)}</div>
              <span className="text-sm truncate">{node.name}</span>
              {node.type === 'schema' && node.children && (
                <Badge variant="secondary" className="ml-auto text-xs">
                  {node.children.length}
                </Badge>
              )}
            </Button>

            {hasChildren && isExpanded && (
              <SchemaTree nodes={node.children!} level={level + 1} />
            )}
          </div>
        )
      })}
    </div>
  )
}
