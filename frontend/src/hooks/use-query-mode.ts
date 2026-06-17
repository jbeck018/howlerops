/**
 * Hook to manage the query editor's single vs multi-DB mode.
 *
 * Mode is stored PER TAB (query-editor-store tab.mode). When the active tab
 * hasn't chosen a mode yet it falls back to auto-detect (multi when more than
 * one connection exists) or an explicit initial mode. This keeps each tab's
 * mode independent and persisted, instead of a single global flag.
 */

import { useShallow } from 'zustand/react/shallow';

import { useConnectionStore } from '@/store/connection-store';
import { useQueryEditorStore } from '@/store/query-editor-store';

export type QueryMode = 'single' | 'multi';

export interface UseQueryModeReturn {
  mode: QueryMode;
  canToggle: boolean;
  toggleMode: () => void;
  connectionCount: number;
  isMultiDB: boolean;
}

export function useQueryMode(initialMode?: 'auto' | QueryMode): UseQueryModeReturn {
  const connectionCount = useConnectionStore((state) => state.connections.length);

  const { activeTabId, activeTabMode, setTabMode } = useQueryEditorStore(
    useShallow((state) => {
      const tab = state.tabs.find((t) => t.id === state.activeTabId);
      return {
        activeTabId: state.activeTabId,
        activeTabMode: tab?.mode,
        setTabMode: state.setTabMode,
      };
    })
  );

  const canToggle = connectionCount > 1;

  const fallback: QueryMode =
    initialMode === 'single' || initialMode === 'multi'
      ? initialMode
      : connectionCount > 1
        ? 'multi'
        : 'single';

  const mode: QueryMode = activeTabMode ?? fallback;

  const toggleMode = () => {
    if (!canToggle || !activeTabId) return;
    setTabMode(activeTabId, mode === 'single' ? 'multi' : 'single');
  };

  return {
    mode,
    canToggle,
    toggleMode,
    connectionCount,
    isMultiDB: mode === 'multi',
  };
}

// Hook to check if multi-DB features should be enabled
export function useMultiDBEnabled(): boolean {
  return useConnectionStore((state) => state.connections.length) > 1;
}
