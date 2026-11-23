import React, { useState, useEffect, useCallback } from 'react';
import { Room, Role, User } from '../types';
import apiService from '../services/api';
import { StorageService } from '../utils/storage';
import { UrlUtils } from '../utils/url';
import ShareModal from './ShareModal';
import './Dashboard.css';

interface DashboardProps {
  currentUser: User | null;
  onOpenRoom: (roomId: string, title: string) => void;
  onOpenAuth: () => void;
}

export const Dashboard: React.FC<DashboardProps> = ({
  currentUser,
  onOpenRoom,
  onOpenAuth,
}) => {
  const [rooms, setRooms] = useState<Room[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [directRoomInput, setDirectRoomInput] = useState('');

  // Create room modal state
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newRoomTitle, setNewRoomTitle] = useState('');
  const [newRoomPublicRole, setNewRoomPublicRole] = useState<Role>('editor');
  const [isCreating, setIsCreating] = useState(false);

  // Share modal state
  const [shareModalRoom, setShareModalRoom] = useState<{ id: string; title: string; role: Role } | null>(null);

  // Copied link toast map
  const [copiedMap, setCopiedMap] = useState<Record<string, boolean>>({});

  // Load documents
  const loadDocuments = useCallback(async () => {
    try {
      setIsLoading(true);
      if (apiService.isAuthenticated()) {
        const fetchedRooms = await apiService.getRooms();
        setRooms(fetchedRooms);
      } else {
        // Fallback to local storage for guest mode
        const localRooms = StorageService.getRooms();
        setRooms(localRooms);
      }
    } catch (err) {
      console.error('Failed to load documents from API, falling back to local storage:', err);
      const localRooms = StorageService.getRooms();
      setRooms(localRooms);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadDocuments();
  }, [loadDocuments, currentUser]);

  // Handle document creation
  const handleCreateDocument = async (e: React.FormEvent) => {
    e.preventDefault();
    const title = newRoomTitle.trim() || 'Untitled Document';

    try {
      setIsCreating(true);
      if (apiService.isAuthenticated()) {
        const newRoom = await apiService.createRoom({ title });
        setIsCreateModalOpen(false);
        setNewRoomTitle('');
        onOpenRoom(newRoom.id, newRoom.title);
      } else {
        // Guest mode creation
        const roomId = UrlUtils.generateRoomId();
        const guestRoom: Room = {
          id: roomId,
          title,
          content: '',
          current_rev: 0,
          public_role: newRoomPublicRole,
        };
        StorageService.addRoom(guestRoom);
        setIsCreateModalOpen(false);
        setNewRoomTitle('');
        onOpenRoom(roomId, title);
      }
    } catch (err) {
      console.error('Failed to create room:', err);
    } finally {
      setIsCreating(false);
    }
  };

  // Handle direct room ID entry
  const handleDirectJoin = (e: React.FormEvent) => {
    e.preventDefault();
    const id = directRoomInput.trim();
    if (id) {
      onOpenRoom(id, 'Shared Document');
    }
  };

  // Copy share URL
  const handleCopyLink = async (roomId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const url = UrlUtils.getShareUrl(roomId);
    await UrlUtils.copyToClipboard(url);
    setCopiedMap((prev) => ({ ...prev, [roomId]: true }));
    setTimeout(() => {
      setCopiedMap((prev) => ({ ...prev, [roomId]: false }));
    }, 2000);
  };

  // Delete room
  const handleDeleteRoom = async (roomId: string, title: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!window.confirm(`Are you sure you want to delete "${title}"? This cannot be undone.`)) {
      return;
    }

    try {
      if (apiService.isAuthenticated()) {
        await apiService.deleteRoom(roomId);
      }
      StorageService.deleteRoom(roomId);
      setRooms((prev) => prev.filter((r) => r.id !== roomId));
    } catch (err) {
      console.error('Failed to delete room:', err);
      alert('Failed to delete document. Ensure you have owner permissions.');
    }
  };

  // Filtered rooms based on search
  const filteredRooms = rooms.filter((r) =>
    (r.title || 'Untitled Document').toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="dashboard-container">
      {/* Dashboard Header Bar */}
      <section className="dashboard-header">
        <div className="dashboard-title-group">
          <h1>Your Documents</h1>
          <p className="dashboard-subtitle">
            {currentUser
              ? `Welcome back, ${currentUser.name}! Create and share documents in real-time.`
              : 'Create and share documents in real-time.'}
          </p>
        </div>

        <div className="dashboard-actions">
          <button
            className="btn btn-primary"
            onClick={() => setIsCreateModalOpen(true)}
          >
            Create New Room
          </button>
        </div>
      </section>

      {/* Search and Direct Join Controls */}
      <section className="dashboard-toolbar">
        <div className="search-bar">
          <span className="search-icon">🔍</span>
          <input
            type="text"
            placeholder="Search documents by title..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="search-input"
          />
          {searchQuery && (
            <button className="clear-search-btn" onClick={() => setSearchQuery('')}>
              ✕
            </button>
          )}
        </div>

        <form onSubmit={handleDirectJoin} className="direct-join-form">
          <input
            type="text"
            placeholder="Enter Room ID or link..."
            value={directRoomInput}
            onChange={(e) => setDirectRoomInput(e.target.value)}
            className="direct-join-input"
          />
          <button type="submit" className="btn btn-secondary" disabled={!directRoomInput.trim()}>
            Join Room
          </button>
        </form>
      </section>

      {/* Guest Notice */}
      {!currentUser && (
        <div className="guest-banner">
          <span>💡 You are using Peeriodic as a guest. </span>
          <button onClick={onOpenAuth} className="btn-link">
            Sign in or register
          </button>
          <span> to save documents permanently to your account across devices.</span>
        </div>
      )}

      {/* Documents Grid */}
      <section className="documents-section">
        {isLoading ? (
          <div className="documents-loading">
            <div className="spinner"></div>
            <p>Loading your documents...</p>
          </div>
        ) : filteredRooms.length === 0 ? (
          <div className="documents-empty">
            <div className="empty-icon">📄</div>
            <h3>{searchQuery ? 'No matching documents found' : 'No documents yet'}</h3>
            <p>
              {searchQuery
                ? 'Try a different search query or clear the filter.'
                : 'Create your first collaborative document to start editing with others in real-time.'}
            </p>
            <button
              className="btn btn-primary"
              onClick={() => setIsCreateModalOpen(true)}
            >
              ➕ Create Document
            </button>
          </div>
        ) : (
          <div className="documents-grid">
            {filteredRooms.map((room) => {
              const isCopied = copiedMap[room.id];
              return (
                <div
                  key={room.id}
                  className="document-card"
                  onClick={() => onOpenRoom(room.id, room.title)}
                >
                  <div className="document-card-header">
                    <span className="doc-icon">📝</span>
                    <div className="doc-badges">
                      {room.public_role && (
                        <span className={`role-badge role-${room.public_role}`}>
                          {room.public_role === 'editor' ? '✏️ Editable' : '👁️ View-only'}
                        </span>
                      )}
                      {room.current_rev !== undefined && room.current_rev > 0 && (
                        <span className="rev-badge">v{room.current_rev}</span>
                      )}
                    </div>
                  </div>

                  <h3 className="document-card-title">{room.title || 'Untitled Document'}</h3>

                  <p className="document-card-preview">
                    {room.content
                      ? room.content.slice(0, 100) + (room.content.length > 100 ? '...' : '')
                      : 'Empty document. Click to open and begin writing...'}
                  </p>

                  <div className="document-card-footer">
                    <span className="doc-date">
                      {room.updated_at
                        ? new Date(room.updated_at).toLocaleDateString(undefined, {
                            month: 'short',
                            day: 'numeric',
                          })
                        : 'Local'}
                    </span>

                    <div className="card-actions-group">
                      <button
                        className="btn-icon"
                        title={isCopied ? 'Link Copied!' : 'Copy Link'}
                        onClick={(e) => handleCopyLink(room.id, e)}
                      >
                        {isCopied ? '✅' : '🔗'}
                      </button>

                      <button
                        className="btn-icon"
                        title="Share & Collaborators"
                        onClick={(e) => {
                          e.stopPropagation();
                          setShareModalRoom({
                            id: room.id,
                            title: room.title,
                            role: 'owner',
                          });
                        }}
                      >
                        👥
                      </button>

                      <button
                        className="btn-icon btn-icon-danger"
                        title="Delete Document"
                        onClick={(e) => handleDeleteRoom(room.id, room.title, e)}
                      >
                        🗑️
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* Create Document Modal */}
      {isCreateModalOpen && (
        <div className="modal-overlay" onClick={() => setIsCreateModalOpen(false)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>Create New Document</h2>
              <button className="modal-close" onClick={() => setIsCreateModalOpen(false)}>
                ✕
              </button>
            </div>

            <form onSubmit={handleCreateDocument} className="create-room-form">
              <div className="form-group">
                <label htmlFor="doc-title">Document Title</label>
                <input
                  id="doc-title"
                  type="text"
                  placeholder="e.g., Sprint Planning Notes"
                  value={newRoomTitle}
                  onChange={(e) => setNewRoomTitle(e.target.value)}
                  autoFocus
                  required
                />
              </div>

              <div className="form-group">
                <label htmlFor="public-role">Default Link Access</label>
                <select
                  id="public-role"
                  value={newRoomPublicRole}
                  onChange={(e) => setNewRoomPublicRole(e.target.value as Role)}
                >
                  <option value="editor">Editor — Anyone with the link can edit</option>
                  <option value="viewer">Viewer — Anyone with the link can read only</option>
                </select>
              </div>

              <div className="modal-actions">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setIsCreateModalOpen(false)}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={isCreating}
                >
                  {isCreating ? 'Creating...' : 'Create Document'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Share Modal */}
      {shareModalRoom && (
        <ShareModal
          isOpen={true}
          roomId={shareModalRoom.id}
          roomName={shareModalRoom.title}
          currentUserRole={shareModalRoom.role}
          onClose={() => setShareModalRoom(null)}
        />
      )}
    </div>
  );
};

export default Dashboard;
