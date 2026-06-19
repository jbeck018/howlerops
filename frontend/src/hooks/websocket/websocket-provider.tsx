/**
 * WebSocket Provider - React provider component for WebSocket context
 */

import React, { useMemo } from 'react';

import {
  UseWebSocketOptions,
  WebSocketContextValue,
} from '../../types/websocket';
import { useWebSocket } from './use-websocket';
import { WebSocketContext } from './websocket-context';

interface WebSocketProviderProps {
  children: React.ReactNode;
  options?: UseWebSocketOptions;
}

/**
 * WebSocket Provider Component
 */
export function WebSocketProvider({ children, options = {} }: WebSocketProviderProps) {
  const webSocket = useWebSocket(options);

  // Depend on the individual (stable) fields rather than the aggregate
  // `webSocket` object, which is a fresh literal every render — depending on it
  // recomputed the memo (and churned the context identity) on every render,
  // re-rendering every consumer. With the callbacks now stable, this only
  // recomputes when connectionState actually changes.
  const contextValue: WebSocketContextValue = useMemo(() => ({
    // Connection
    socket: webSocket.getSocket(),
    connectionState: webSocket.connectionState,
    connect: webSocket.connect,
    disconnect: webSocket.disconnect,

    // Rooms
    joinRoom: webSocket.joinRoom,
    leaveRoom: webSocket.leaveRoom,
    getRooms: webSocket.getRooms,

    // Messaging
    sendMessage: webSocket.sendMessage,

    // Event handling
    on: webSocket.on,
    off: webSocket.off,

    // Utilities
    getStats: webSocket.getStats,
    healthCheck: webSocket.healthCheck,
  }), [
    webSocket.connectionState,
    webSocket.connect,
    webSocket.disconnect,
    webSocket.joinRoom,
    webSocket.leaveRoom,
    webSocket.getRooms,
    webSocket.sendMessage,
    webSocket.on,
    webSocket.off,
    webSocket.getStats,
    webSocket.healthCheck,
    webSocket.getSocket,
  ]);

  return (
    <WebSocketContext.Provider value={contextValue}>
      {children}
    </WebSocketContext.Provider>
  );
}