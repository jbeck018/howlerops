import React, { useState, useCallback, useMemo, useRef, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { 
  ChevronRight, 
  ChevronDown, 
  Edit3, 
  Check, 
  X, 
  AlertCircle,
  Copy,
  Eye,
  EyeOff
} from 'lucide-react'
import { JsonToken, getTokenClass } from '@/lib/json-formatter'
import { SearchMatch, highlightMatches } from '@/lib/json-search'
import { CellValue } from '@/types/table'
import { ForeignKeyResolver } from './foreign-key-resolver'

interface JsonEditorProps {
  tokens: JsonToken[]
  data: Record<string, CellValue>
  isEditing: boolean
  validationErrors: Map<string, string>
  searchMatches: SearchMatch[]
  currentMatchIndex: number
  wordWrap: boolean
  expandedKeys: Set<string>
  collapsedKeys: Set<string>
  onToggleEdit: () => void
  onUpdateField: (key: string, value: CellValue) => void
  onToggleKeyExpansion: (key: string) => void
  onCopyJson: () => void
  metadata?: any
  connectionId?: string
}

interface JsonNode {
  key: string
  value: CellValue
  path: string
  level: number
  isExpanded: boolean
  isEditing: boolean
  validationError?: string
}

export function JsonEditor({
  tokens,
  data,
  isEditing,
  validationErrors,
  searchMatches,
  currentMatchIndex,
  wordWrap,
  expandedKeys,
  collapsedKeys,
  onToggleEdit,
  onUpdateField,
  onToggleKeyExpansion,
  onCopyJson,
  metadata,
  connectionId
}: JsonEditorProps) {
  const [editingField, setEditingField] = useState<string | null>(null)
  const [editValue, setEditValue] = useState<string>('')
  const editInputRef = useRef<HTMLInputElement>(null)

  // Parse JSON data into hierarchical structure
  const jsonNodes = useMemo((): JsonNode[] => {
    const nodes: JsonNode[] = []
    
    const processObject = (obj: Record<string, CellValue>, path: string = '', level: number = 0) => {
      Object.entries(obj).forEach(([key, value]) => {
        if (key === '__rowId') return
        
        const currentPath = path ? `${path}.${key}` : key
        const isExpanded = expandedKeys.has('*') || expandedKeys.has(currentPath)
        const isCollapsed = collapsedKeys.has('*') || collapsedKeys.has(currentPath)
        const validationError = validationErrors.get(key)
        
        nodes.push({
          key,
          value,
          path: currentPath,
          level,
          isExpanded: isExpanded && !isCollapsed,
          isEditing: editingField === key,
          validationError
        })
      })
    }
    
    processObject(data)
    return nodes
  }, [data, expandedKeys, collapsedKeys, validationErrors, editingField])

  // Handle field editing
  const handleStartEdit = useCallback((key: string, currentValue: CellValue) => {
    setEditingField(key)
    setEditValue(String(currentValue ?? ''))
  }, [])

  const handleSaveEdit = useCallback(() => {
    if (!editingField) return
    
    onUpdateField(editingField, editValue)
    setEditingField(null)
    setEditValue('')
  }, [editingField, editValue, onUpdateField])

  const handleCancelEdit = useCallback(() => {
    setEditingField(null)
    setEditValue('')
  }, [])

  // Focus edit input when editing starts
  useEffect(() => {
    if (editingField && editInputRef.current) {
      editInputRef.current.focus()
      editInputRef.current.select()
    }
  }, [editingField])

  // Handle keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleCancelEdit()
      } else if (e.key === 'Enter' && editingField) {
        handleSaveEdit()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [editingField, handleSaveEdit, handleCancelEdit])

  // Render JSON tokens with syntax highlighting
  const renderTokens = useCallback(() => {
    if (searchMatches.length > 0) {
      // Highlight search matches
      const highlighted = highlightMatches(
        tokens.map(t => t.value).join(''),
        searchMatches,
        currentMatchIndex
      )
      return (
        <div 
          className="font-mono text-sm whitespace-pre"
          dangerouslySetInnerHTML={{ __html: highlighted }}
        />
      )
    }

    return (
      <div className="font-mono text-sm whitespace-pre">
        {tokens.map((token, index) => (
          <span
            key={index}
            className={getTokenClass(token)}
            style={{ wordBreak: wordWrap ? 'break-word' : 'normal' }}
          >
            {token.value}
          </span>
        ))}
      </div>
    )
  }, [tokens, searchMatches, currentMatchIndex, wordWrap])

  // Render individual field
  const renderField = useCallback((node: JsonNode) => {
    const { key, value, level, isExpanded, isEditing, validationError } = node
    
    return (
      <div key={key} className="json-field" style={{ marginLeft: `${level * 20}px` }}>
        <div className="flex items-center gap-2 py-1">
          {/* Expand/collapse button for objects/arrays */}
          {(typeof value === 'object' && value !== null) && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onToggleKeyExpansion(node.path)}
              className="h-4 w-4 p-0"
            >
              {isExpanded ? (
                <ChevronDown className="h-3 w-3" />
              ) : (
                <ChevronRight className="h-3 w-3" />
              )}
            </Button>
          )}
          
          {/* Key name */}
          <span className="text-blue-600 font-medium">{key}:</span>
          
          {/* Value */}
          {isEditing ? (
            <div className="flex items-center gap-2 flex-1">
              <Input
                ref={editInputRef}
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                className="h-6 text-xs"
                size={1}
              />
              <Button
                variant="ghost"
                size="sm"
                onClick={handleSaveEdit}
                className="h-6 w-6 p-0 text-green-600"
              >
                <Check className="h-3 w-3" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleCancelEdit}
                className="h-6 w-6 p-0 text-red-600"
              >
                <X className="h-3 w-3" />
              </Button>
            </div>
          ) : (
            <div className="flex items-center gap-2 flex-1">
              <span className={getValueClass(value)}>
                {formatValue(value)}
              </span>
              
              {/* Edit button */}
              {isEditing && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleStartEdit(key, value)}
                  className="h-4 w-4 p-0 opacity-0 group-hover:opacity-100"
                >
                  <Edit3 className="h-3 w-3" />
                </Button>
              )}
              
              {/* Validation error */}
              {validationError && (
                <Badge variant="destructive" className="text-xs">
                  <AlertCircle className="h-3 w-3 mr-1" />
                  {validationError}
                </Badge>
              )}
            </div>
          )}
        </div>
        
        {/* Foreign key resolver */}
          <ForeignKeyResolver
            key={`fk-${key}`}
            value={value}
          metadata={metadata}
          connectionId={connectionId}
          isExpanded={isExpanded}
          onToggle={onToggleKeyExpansion}
          onLoadData={() => Promise.resolve()}
        />
      </div>
    )
  }, [isEditing, onToggleKeyExpansion, handleStartEdit, handleSaveEdit, handleCancelEdit, editValue, metadata, connectionId])

  return (
    <div className="json-editor">
      {/* Toolbar */}
      <div className="flex items-center justify-between p-2 border-b">
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={onCopyJson}
            className="h-7"
          >
            <Copy className="h-3 w-3 mr-1" />
            Copy JSON
          </Button>
        </div>
        
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {searchMatches.length > 0 && (
            <span>
              {currentMatchIndex + 1} of {searchMatches.length} matches
            </span>
          )}
        </div>
      </div>

      {/* JSON Content */}
      <ScrollArea className="h-full">
        <div className="p-4 space-y-1">
          {isEditing ? (
            // Tree view for editing
            <div className="space-y-1">
              {jsonNodes.map(renderField)}
            </div>
          ) : (
            // Syntax highlighted view
            <div className="json-content">
              {renderTokens()}
            </div>
          )}
        </div>
      </ScrollArea>
    </div>
  )
}

/**
 * Get CSS class for value based on type
 */
function getValueClass(value: CellValue): string {
  if (value === null) return 'text-gray-500'
  if (typeof value === 'boolean') return 'text-orange-600'
  if (typeof value === 'number') return 'text-purple-600'
  if (typeof value === 'string') return 'text-green-600'
  if (typeof value === 'object') return 'text-gray-700'
  return 'text-gray-900'
}

/**
 * Format value for display
 */
function formatValue(value: CellValue): string {
  if (value === null) return 'null'
  if (value === undefined) return 'undefined'
  if (typeof value === 'boolean') return value.toString()
  if (typeof value === 'number') return value.toString()
  if (typeof value === 'string') return `"${value}"`
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value)
    } catch {
      return '[Object]'
    }
  }
  return String(value)
}

/**
 * Hook for managing JSON editor state
 */
export function useJsonEditor() {
  const [isEditing, setIsEditing] = useState(false)
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set())
  const [collapsedKeys, setCollapsedKeys] = useState<Set<string>>(new Set())

  const toggleEdit = useCallback(() => {
    setIsEditing(prev => !prev)
  }, [])

  const toggleKeyExpansion = useCallback((key: string) => {
    setExpandedKeys(prev => {
      const newSet = new Set(prev)
      if (newSet.has(key)) {
        newSet.delete(key)
        setCollapsedKeys(prevCollapsed => new Set(prevCollapsed).add(key))
      } else {
        newSet.add(key)
        setCollapsedKeys(prevCollapsed => {
          const newCollapsed = new Set(prevCollapsed)
          newCollapsed.delete(key)
          return newCollapsed
        })
      }
      return newSet
    })
  }, [])

  const expandAllKeys = useCallback(() => {
    setExpandedKeys(new Set(['*']))
    setCollapsedKeys(new Set())
  }, [])

  const collapseAllKeys = useCallback(() => {
    setExpandedKeys(new Set())
    setCollapsedKeys(new Set(['*']))
  }, [])

  return {
    isEditing,
    expandedKeys,
    collapsedKeys,
    toggleEdit,
    toggleKeyExpansion,
    expandAllKeys,
    collapseAllKeys
  }
}
