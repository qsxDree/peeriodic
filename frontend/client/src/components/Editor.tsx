import React, { useState, useEffect, useCallback, useRef } from 'react';
import { WebSocketMessage } from '../types';
import { ClientOTState } from '../utils/ot';
import apiService from '../services/api';
import wsService from '../services/websocket';
import { useDocStore } from '../store/docStore';
import CodeMirrorEditor from './CodeMirrorEditor';
import OfflineBanner from './OfflineBanner';
import ShareModal from './ShareModal';
import './Editor.css';

interface EditorProps {
  roomId: string;
  roomName: string;
  onBack: () => void;
  onShare: () => void;
}

const Editor: React.FC<EditorProps> = ({ roomId, roomName, onBack, onShare }) => {
  // Global Store State
  const content = useDocStore((s) => s.content);
  const rev = useDocStore((s) => s.rev);
  const role = useDocStore((s) => s.role);
  const isLoading = useDocStore((s) => s.isLoading);
  const isSaving = useDocStore((s) => s.isSaving);
  const connectionStatus = useDocStore((s) => s.connectionStatus);
  const presences = useDocStore((s) => s.presences);
  const myName = useDocStore((s) => s.myName);
  const myColor = useDocStore((s) => s.myColor);
  const myClientId = useDocStore((s) => s.myClientId);

  const initDocument = useDocStore((s) => s.initDocument);
  const setContent = useDocStore((s) => s.setContent);
  const setRole = useDocStore((s) => s.setRole);
  const setIsLoading = useDocStore((s) => s.setIsLoading);
  const setIsSaving = useDocStore((s) => s.setIsSaving);
  const setConnectionStatus = useDocStore((s) => s.setConnectionStatus);
  const setPendingOpsCount = useDocStore((s) => s.setPendingOpsCount);
  const setMySession = useDocStore((s) => s.setMySession);
  const setPresences = useDocStore((s) => s.setPresences);
  const updateRemoteCursor = useDocStore((s) => s.updateRemoteCursor);
  const removeRemoteCursor = useDocStore((s) => s.removeRemoteCursor);

  const [isShareModalOpen, setIsShareModalOpen] = useState(false);
  const otStateRef = useRef<ClientOTState>(new ClientOTState('', 0));
  const myClientIdRef = useRef<string>('');
  const autoSaveIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const isViewer = role === 'viewer';

  // 1. Fetch initial document content & user role from REST API
  useEffect(() => {
    let isCancelled = false;

    const loadDocument = async () => {
      try {
        setIsLoading(true);
        const response = await apiService.getDocument(roomId);
        if (isCancelled) return;

        const docContent = response.content || '';
        const docRev = response.current_rev || 0;
        const userRole = response.role || 'editor';

        otStateRef.current = new ClientOTState(docContent, docRev);
        initDocument(roomId, roomName, docContent, docRev, userRole);
      } catch (error) {
        console.error('Failed to load document via API:', error);
      } finally {
        if (!isCancelled) setIsLoading(false);
      }
    };

    loadDocument();
    return () => {
      isCancelled = true;
    };
  }, [roomId, roomName, initDocument, setIsLoading]);

  // 2. Setup WebSocket connection and handle collaboration protocol
  useEffect(() => {
    let isActive = true;

    // Track real-time connection status
    wsService.onStatusChange((status) => {
      if (isActive) {
        setConnectionStatus(status);
        if (status === 'connected' && otStateRef.current.rev > 0) {
          wsService.sendSync(otStateRef.current.rev);
        }
      }
    });

    // Handle incoming WebSocket messages
    wsService.onMessage((message: WebSocketMessage) => {
      if (!isActive) return;

      switch (message.type) {
        case 'init':
          if (message.data !== undefined) {
            const serverRev = message.rev || 0;
            otStateRef.current = new ClientOTState(message.data, serverRev);
            setContent(message.data, serverRev);
          }
          if (message.client_id) {
            myClientIdRef.current = message.client_id;
          }
          if (message.name && message.color) {
            setMySession(message.client_id || '', message.name, message.color, message.role);
          }
          if (message.role) {
            setRole(message.role);
          }
          if (message.presence) {
            setPresences(message.presence);
          }
          break;

        case 'ack':
          if (message.rev !== undefined) {
            const bufferedOpsToSend = otStateRef.current.handleServerAck(message.rev);
            setPendingOpsCount(otStateRef.current.getPendingCount());
            if (bufferedOpsToSend && bufferedOpsToSend.length > 0) {
              wsService.sendOp(message.rev, bufferedOpsToSend);
            }
          }
          break;

        case 'op':
          if (message.client_id !== myClientIdRef.current && message.ops && message.rev !== undefined) {
            const { newDoc } = otStateRef.current.handleServerOp(message.rev, message.ops);
            setContent(newDoc, message.rev);
          }
          break;

        case 'cursor':
          if (message.client_id && message.client_id !== myClientIdRef.current && message.cursor) {
            updateRemoteCursor(
              message.client_id,
              message.name || 'User',
              message.color || '#e06c75',
              message.cursor
            );
          }
          break;

        case 'presence':
          if (message.presence) {
            setPresences(message.presence);
            const activeIds = new Set(message.presence.map((p) => p.client_id));
            useDocStore.getState().presences.forEach((p) => {
              if (!activeIds.has(p.client_id)) {
                removeRemoteCursor(p.client_id);
              }
            });
          }
          break;

        case 'sync':
          if (message.rev !== undefined) {
            const resyncedDoc = otStateRef.current.handleSync(message.rev, message.data, message.ops);
            setContent(resyncedDoc, message.rev);
            setPendingOpsCount(0);
          }
          break;

        case 'error':
          console.warn('Real-time collaboration error:', message.data);
          break;

        default:
          break;
      }
    });

    wsService.connect(roomId).catch((err) => {
      console.error('WebSocket connection failure:', err);
    });

    return () => {
      isActive = false;
      wsService.clearHandlers();
      wsService.disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roomId]);

  // 3. Document Saving
  const handleSave = useCallback(async () => {
    if (!content.trim() || isViewer) return;

    try {
      setIsSaving(true);
      await apiService.saveDocument(roomId, content);
    } catch (error) {
      console.error('Failed to save document:', error);
    } finally {
      setIsSaving(false);
    }
  }, [content, roomId, isViewer, setIsSaving]);

  // 4. Periodic Auto-save (every 30 seconds if user has write access)
  useEffect(() => {
    if (content.trim() && !isViewer) {
      autoSaveIntervalRef.current = setInterval(() => {
        handleSave();
      }, 30000);
    }

    return () => {
      if (autoSaveIntervalRef.current) {
        clearInterval(autoSaveIntervalRef.current);
      }
    };
  }, [content, handleSave, isViewer]);

  // 5. Handle local edit from CodeMirror
  const handleLocalChange = useCallback(
    (newContent: string) => {
      if (isViewer) return;

      const opsToSend = otStateRef.current.applyLocalEdit(newContent);
      setPendingOpsCount(otStateRef.current.getPendingCount());

      if (opsToSend && opsToSend.length > 0 && wsService.isConnected()) {
        wsService.sendOp(otStateRef.current.rev, opsToSend);
      }
    },
    [isViewer, setPendingOpsCount]
  );

  const getStatusBadge = () => {
    switch (connectionStatus) {
      case 'connected':
        return <span className="status-badge status-connected">● Connected</span>;
      case 'reconnecting':
        return <span className="status-badge status-reconnecting">⟳ Reconnecting...</span>;
      case 'connecting':
        return <span className="status-badge status-connecting">◌ Connecting...</span>;
      case 'disconnected':
      default:
        return <span className="status-badge status-disconnected">○ Offline</span>;
    }
  };

  const peerCount = presences.length > 0 ? presences.length : 1;
  const wordCount = content.trim() ? content.trim().split(/\s+/).length : 0;
  const charCount = Array.from(content).length;

  return (
    <div className="editor">
      {/* Editor Top Navigation Header */}
      <header className="editor-header">
        <div className="editor-title">
          <h2>{roomName}</h2>
          {getStatusBadge()}
          <span className={`role-badge role-${role}`}>
            {role === 'owner' ? '👑 Owner' : role === 'editor' ? '✏️ Editor' : '👁️ Viewer'}
          </span>
          <span className="rev-badge">v{rev}</span>
          {myName && (
            <span className="user-name-badge" style={{ borderColor: myColor, color: myColor }}>
              {myName}
            </span>
          )}
        </div>

        {/* Live Collaborator Presence Avatars */}
        <div className="presence-bar">
          <div className="presence-avatars">
            {presences.map((user) => {
              const isMe = user.client_id === myClientId;
              return (
                <div
                  key={user.client_id}
                  className={`presence-avatar ${isMe ? 'is-me' : ''}`}
                  style={{ backgroundColor: user.color }}
                  title={`${user.name}${user.role ? ` (${user.role})` : ''}${isMe ? ' (You)' : ''}`}
                >
                  {user.name ? user.name.charAt(0).toUpperCase() : 'U'}
                </div>
              );
            })}
          </div>
          <span className="peer-count-text">
            {peerCount} {peerCount === 1 ? 'collaborator' : 'collaborators'}
          </span>
        </div>

        {/* Header Actions */}
        <div className="editor-actions">
          <button onClick={onBack} className="btn btn-back">
            ⬅ Dashboard
          </button>
          <button
            onClick={() => {
              setIsShareModalOpen(true);
              onShare();
            }}
            className="btn btn-share"
          >
            👥 Share
          </button>
          <button
            onClick={handleSave}
            disabled={isSaving || isViewer}
            className={`btn btn-save ${isSaving ? 'btn-saving' : ''}`}
            title={isViewer ? 'Read-only viewer cannot save changes' : 'Save document'}
          >
            {isSaving ? '⏳ Saving...' : '💾 Save'}
          </button>
        </div>
      </header>

      {/* Offline & Reconnection Notifications */}
      <OfflineBanner />

      {/* Viewer Read-Only Banner */}
      {isViewer && (
        <div className="viewer-notice-banner">
          👁️ <strong>Read-Only Mode:</strong> You have viewer permissions for this document. Contact the owner to request editing access.
        </div>
      )}

      {/* Editor Body */}
      <main className="editor-main">
        {isLoading ? (
          <div className="loading-indicator">
            <div className="spinner"></div>
            <p>Loading document content...</p>
          </div>
        ) : (
          <div className="editor-workspace">
            <CodeMirrorEditor
              onDocChange={handleLocalChange}
              readOnly={isViewer}
            />

            {/* Editor Footer Status Bar */}
            <footer className="editor-status-bar">
              <span className="status-item">Markdown Enabled</span>
              <span className="status-divider">•</span>
              <span className="status-item">{wordCount} words</span>
              <span className="status-divider">•</span>
              <span className="status-item">{charCount} characters</span>
              <span className="status-divider">•</span>
              <span className="status-item">Revision {rev}</span>
            </footer>
          </div>
        )}
      </main>

      {/* Share & Role Management Modal */}
      <ShareModal
        isOpen={isShareModalOpen}
        roomId={roomId}
        roomName={roomName}
        currentUserRole={role}
        onClose={() => setIsShareModalOpen(false)}
      />
    </div>
  );
};

export default Editor;
