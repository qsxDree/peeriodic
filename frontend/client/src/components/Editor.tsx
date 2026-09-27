import React, { useState, useEffect, useCallback, useRef } from 'react';
import { WebSocketMessage, UserPresence, CursorPosition, Role } from '../types';
import { ClientOTState, transformCursor } from '../utils/ot';
import apiService from '../services/api';
import wsService, { ConnectionStatus } from '../services/websocket';
import ShareModal from './ShareModal';
import './Editor.css';

interface EditorProps {
  roomId: string;
  roomName: string;
  onBack: () => void;
  onShare: () => void;
}

const Editor: React.FC<EditorProps> = ({ roomId, roomName, onBack, onShare }) => {
  const [document, setDocument] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('disconnected');
  const [presenceList, setPresenceList] = useState<UserPresence[]>([]);
  const [myColor, setMyColor] = useState<string>('#007bff');
  const [myName, setMyName] = useState<string>('');
  const [currentUserRole, setCurrentUserRole] = useState<Role>('editor');
  const [remoteCursors, setRemoteCursors] = useState<Record<string, { name: string; color: string; cursor: CursorPosition }>>({});
  const [isShareModalOpen, setIsShareModalOpen] = useState(false);

  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const otStateRef = useRef<ClientOTState>(new ClientOTState('', 0));
  const myClientIdRef = useRef<string>('');
  const autoSaveIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const isViewer = currentUserRole === 'viewer';

  // Load initial document content and role from REST
  useEffect(() => {
    let isCancelled = false;

    const loadDocument = async () => {
      try {
        setIsLoading(true);
        const response = await apiService.getDocument(roomId);
        if (isCancelled) return;
        const content = response.content || '';
        const rev = response.current_rev || 0;
        if (response.role) {
          setCurrentUserRole(response.role);
        }
        otStateRef.current = new ClientOTState(content, rev);
        setDocument(content);
      } catch (error) {
        console.error('Failed to load document:', error);
      } finally {
        if (!isCancelled) setIsLoading(false);
      }
    };

    loadDocument();
    return () => {
      isCancelled = true;
    };
  }, [roomId]);

  // Setup WebSocket connection and OT protocol handlers
  useEffect(() => {
    let isActive = true;

    // Track connection status
    wsService.onStatusChange((status) => {
      if (isActive) {
        setConnectionStatus(status);
        if (status === 'connected' && otStateRef.current.rev > 0) {
          wsService.sendSync(otStateRef.current.rev);
        }
      }
    });

    // Handle WebSocket messages
    wsService.onMessage((message: WebSocketMessage) => {
      if (!isActive) return;

      switch (message.type) {
        case 'init':
          if (message.data !== undefined) {
            const rev = message.rev || 0;
            otStateRef.current = new ClientOTState(message.data, rev);
            setDocument(message.data);
          }
          if (message.client_id) {
            myClientIdRef.current = message.client_id;
          }
          if (message.color) {
            setMyColor(message.color);
          }
          if (message.name) {
            setMyName(message.name);
          }
          if (message.role) {
            setCurrentUserRole(message.role);
          }
          if (message.presence) {
            setPresenceList(message.presence);
          }
          break;

        case 'ack':
          if (message.rev !== undefined) {
            const bufferedOpsToSend = otStateRef.current.handleServerAck(message.rev);
            if (bufferedOpsToSend && bufferedOpsToSend.length > 0) {
              wsService.sendOp(message.rev, bufferedOpsToSend);
            }
          }
          break;

        case 'op':
          if (message.client_id !== myClientIdRef.current && message.ops && message.rev !== undefined) {
            const currentSelection = textareaRef.current
              ? { start: textareaRef.current.selectionStart, end: textareaRef.current.selectionEnd }
              : null;

            const { newDoc, transformedRemoteOps } = otStateRef.current.handleServerOp(message.rev, message.ops);
            setDocument(newDoc);

            if (currentSelection && textareaRef.current) {
              let newStart = currentSelection.start;
              let newEnd = currentSelection.end;
              for (const op of transformedRemoteOps) {
                newStart = transformCursor(newStart, op);
                newEnd = transformCursor(newEnd, op);
              }
              requestAnimationFrame(() => {
                if (textareaRef.current) {
                  textareaRef.current.setSelectionRange(newStart, newEnd);
                }
              });
            }
          }
          break;

        case 'cursor':
          if (message.client_id && message.client_id !== myClientIdRef.current && message.cursor) {
            setRemoteCursors(prev => ({
              ...prev,
              [message.client_id!]: {
                name: message.name || 'User',
                color: message.color || '#e06c75',
                cursor: message.cursor!,
              },
            }));
          }
          break;

        case 'presence':
          if (message.presence) {
            setPresenceList(message.presence);
            const activeIds = new Set(message.presence.map(p => p.client_id));
            setRemoteCursors(prev => {
              const updated = { ...prev };
              Object.keys(updated).forEach(id => {
                if (!activeIds.has(id)) delete updated[id];
              });
              return updated;
            });
          }
          break;

        case 'sync':
          if (message.rev !== undefined) {
            const resyncedDoc = otStateRef.current.handleSync(message.rev, message.data, message.ops);
            setDocument(resyncedDoc);
          }
          break;

        case 'error':
          console.warn('WebSocket error received from server:', message.data);
          break;

        default:
          break;
      }
    });

    wsService.connect(roomId).catch(err => {
      console.error('WebSocket connection failure:', err);
    });

    return () => {
      isActive = false;
      wsService.clearHandlers();
      wsService.disconnect();
    };
  }, [roomId]);

  const handleSave = useCallback(async () => {
    if (!document.trim() || isViewer) return;

    try {
      setIsSaving(true);
      await apiService.saveDocument(roomId, document);
    } catch (error) {
      console.error('Failed to save document:', error);
    } finally {
      setIsSaving(false);
    }
  }, [document, roomId, isViewer]);

  // Periodic Auto-save (only if user has write permissions)
  useEffect(() => {
    if (document.trim() && !isViewer) {
      autoSaveIntervalRef.current = setInterval(() => {
        handleSave();
      }, 30000);
    }

    return () => {
      if (autoSaveIntervalRef.current) {
        clearInterval(autoSaveIntervalRef.current);
      }
    };
  }, [document, handleSave, isViewer]);

  // Handle local text edit with OT
  const handleDocumentChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    if (isViewer) return;

    const newContent = e.target.value;
    setDocument(newContent);

    const opsToSend = otStateRef.current.applyLocalEdit(newContent);
    if (opsToSend && opsToSend.length > 0 && wsService.isConnected()) {
      wsService.sendOp(otStateRef.current.rev, opsToSend);
    }
  }, [isViewer]);

  // Broadcast cursor and selection changes
  const handleCursorMove = useCallback(() => {
    if (!textareaRef.current || !wsService.isConnected()) return;

    const pos = textareaRef.current.selectionStart;
    const selStart = textareaRef.current.selectionStart;
    const selEnd = textareaRef.current.selectionEnd;

    wsService.sendCursor({
      pos,
      sel_start: selStart,
      sel_end: selEnd,
    });
  }, []);

  const handleShareClick = () => {
    setIsShareModalOpen(true);
    onShare();
  };

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

  const peerCount = presenceList.length > 0 ? presenceList.length : 1;

  return (
    <div className="editor">
      <header className="editor-header">
        <div className="editor-title">
          <h2>{roomName}</h2>
          {getStatusBadge()}
          <span className={`role-badge role-${currentUserRole}`}>
            {currentUserRole === 'owner' ? '👑 Owner' : currentUserRole === 'editor' ? '✏️ Editor' : '👁️ Viewer'}
          </span>
          <span className="rev-badge">rev {otStateRef.current.rev}</span>
          {myName && (
            <span className="user-name-badge" style={{ borderColor: myColor, color: myColor }}>
              {myName}
            </span>
          )}
        </div>

        {/* Live Presence Avatars */}
        <div className="presence-bar">
          <div className="presence-avatars">
            {presenceList.map((user) => {
              const isMe = user.client_id === myClientIdRef.current;
              return (
                <div
                  key={user.client_id}
                  className={`presence-avatar ${isMe ? 'is-me' : ''}`}
                  style={{ backgroundColor: user.color }}
                  title={`${user.name}${user.role ? ` (${user.role})` : ''}${isMe ? ' (You)' : ''}`}
                >
                  {user.name.charAt(0).toUpperCase()}
                </div>
              );
            })}
          </div>
          <span className="peer-count-text">
            {peerCount} {peerCount === 1 ? 'collaborator' : 'collaborators'}
          </span>
        </div>

        <div className="editor-actions">
          <button onClick={onBack} className="btn btn-back">
            ⬅ Back
          </button>
          <button onClick={handleShareClick} className="btn btn-share">
            🔗 Share
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

      {isViewer && (
        <div className="viewer-notice-banner">
          👁️ <strong>Read-Only Mode:</strong> You have viewer access to this document. Contact the owner to request editor permissions.
        </div>
      )}

      <main className="editor-main">
        {isLoading ? (
          <div className="loading-indicator">
            <div className="spinner"></div>
            <p>Loading document...</p>
          </div>
        ) : (
          <div className="editor-container">
            <textarea
              ref={textareaRef}
              value={document}
              onChange={handleDocumentChange}
              onKeyUp={handleCursorMove}
              onClick={handleCursorMove}
              onSelect={handleCursorMove}
              readOnly={isViewer}
              placeholder={isViewer ? "This document is read-only..." : "Start typing collaboratively in real-time..."}
              className={`document-editor ${isViewer ? 'read-only' : ''}`}
              rows={24}
              cols={80}
            />

            {/* Remote Cursors and Presence Info Footer */}
            <div className="remote-cursors-info">
              {Object.entries(remoteCursors).map(([id, info]) => (
                <span
                  key={id}
                  className="remote-cursor-pill"
                  style={{ borderLeftColor: info.color }}
                >
                  <span className="cursor-dot" style={{ backgroundColor: info.color }}></span>
                  {info.name} at line {document.substring(0, info.cursor.pos).split('\n').length}
                </span>
              ))}
            </div>
          </div>
        )}
      </main>

      {/* Share & Invite Management Modal */}
      <ShareModal
        isOpen={isShareModalOpen}
        roomId={roomId}
        roomName={roomName}
        currentUserRole={currentUserRole}
        onClose={() => setIsShareModalOpen(false)}
      />
    </div>
  );
};

export default Editor;
