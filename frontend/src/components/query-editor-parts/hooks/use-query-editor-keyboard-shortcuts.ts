import { useEffect } from "react"

import { type CodeMirrorEditorRef } from "@/components/codemirror-editor"
import { type DatabaseConnection } from "@/store/connection-store"
import type { QueryTab } from "@/store/query-types"
import { buildExecutableSql } from "@/utils/sql"

interface User {
  id: string
}

interface UseQueryEditorKeyboardShortcutsParams {
  editorRef: React.RefObject<CodeMirrorEditorRef | null>
  editorContentRef: React.MutableRefObject<string>
  editorContent: string
  user: User | null
  activeConnection: DatabaseConnection | null
  activeTab: QueryTab | undefined
  setShowDiagnostics: React.Dispatch<React.SetStateAction<boolean>>
  setShowSavedQueries: React.Dispatch<React.SetStateAction<boolean>>
  setShowSaveQueryDialog: React.Dispatch<React.SetStateAction<boolean>>
  handleExecuteQuery: () => void
  handleCreateSqlTab: () => void
  handleCreateAiTab: () => void
}

export function useQueryEditorKeyboardShortcuts({
  editorRef,
  editorContentRef,
  editorContent,
  user,
  activeConnection,
  activeTab,
  setShowDiagnostics,
  setShowSavedQueries,
  setShowSaveQueryDialog,
  handleExecuteQuery,
  handleCreateSqlTab,
  handleCreateAiTab,
}: UseQueryEditorKeyboardShortcutsParams): void {
  // Keyboard shortcuts:
  // - Ctrl/Cmd+Shift+D: toggle diagnostics
  // - Ctrl/Cmd+Shift+L: open Saved Queries library
  // - Ctrl/Cmd+Shift+S: open Save Query dialog
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'D') {
        e.preventDefault()
        setShowDiagnostics(prev => !prev)
      }

      if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'L' || e.key === 'l')) {
        e.preventDefault()
        setShowSavedQueries(prev => !prev)
      }

      if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'S' || e.key === 's')) {
        if ((editorRef.current?.getValue() ?? editorContentRef.current).trim()) {
          e.preventDefault()
          setShowSaveQueryDialog(true)
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Keyboard shortcut for executing query (Ctrl/Cmd+Enter)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        const view = editorRef.current?.getView()
        if (view && e.target instanceof Node && view.contentDOM.contains(e.target)) {
          // Let the editor's own keymap handle execution to avoid double-triggering
          return
        }

        e.preventDefault()

        if (activeConnection?.isConnected && !activeTab?.isExecuting) {
          const currentEditorValue = editorRef.current?.getValue() ?? editorContent
          const selectedText = editorRef.current?.getSelectedText?.() ?? ''
          const cursorOffset = editorRef.current?.getCursorOffset?.() ?? currentEditorValue.length
          if (selectedText.trim().length > 0) {
            handleExecuteQuery()
            return
          }
          const executableQuery = buildExecutableSql(currentEditorValue, {
            selectionText: selectedText,
            cursorOffset,
          })

          if (executableQuery) {
            handleExecuteQuery()
          }
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [activeConnection, editorContent, activeTab?.isExecuting, handleExecuteQuery, editorRef])

  // Keyboard shortcut for saving query (Ctrl/Cmd+Shift+S)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'S') {
        e.preventDefault()

        // Only open save dialog if user is authenticated and query has content
        if (user && editorContent.trim()) {
          setShowSaveQueryDialog(true)
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [user, editorContent, setShowSaveQueryDialog])

  // Keyboard shortcut for opening Saved Queries (Ctrl/Cmd+Shift+L)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toUpperCase() === 'L') {
        e.preventDefault()
        if (user) setShowSavedQueries(true)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [user, setShowSavedQueries])

  useEffect(() => {
    const isTypingTarget = (target: EventTarget | null) => {
      if (!(target instanceof HTMLElement)) {
        return false
      }
      const tag = target.tagName.toLowerCase()
      return tag === 'input' || tag === 'textarea' || target.isContentEditable
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (isTypingTarget(event.target)) {
        return
      }

      const modifierPressed = event.metaKey || event.ctrlKey
      if (!modifierPressed || !event.shiftKey) {
        return
      }

      const key = event.key.toLowerCase()
      if (key === 'n') {
        event.preventDefault()
        handleCreateSqlTab()
      } else if (key === 'g') {
        event.preventDefault()
        handleCreateAiTab()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [handleCreateSqlTab, handleCreateAiTab])
}
