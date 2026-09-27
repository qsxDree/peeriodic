import React, { useState, useEffect, useCallback } from 'react';
import apiService from '../services/api';
import { UrlUtils } from '../utils/url';
import { DocumentMember, Role, RoomInvite } from '../types';
import './ShareModal.css';

interface ShareModalProps {
  isOpen: boolean;
  roomId: string;
  roomName: string;
  currentUserRole: Role;
  onClose: () => void;
}

const ShareModal: React.FC<ShareModalProps> = ({
  isOpen,
  roomId,
  roomName,
  currentUserRole,
  onClose,
}) => {
  const [inviteRole, setInviteRole] = useState<Role>('editor');
  const [generatedInvite, setGeneratedInvite] = useState<RoomInvite | null>(null);
  const [copiedDirect, setCopiedDirect] = useState(false);
  const [copiedInvite, setCopiedInvite] = useState(false);
  const [members, setMembers] = useState<DocumentMember[]>([]);
  const [isLoadingMembers, setIsLoadingMembers] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);

  const loadMembers = useCallback(async () => {
    try {
      setIsLoadingMembers(true);
      const data = await apiService.getRoomMembers(roomId);
      setMembers(data);
    } catch (err) {
      console.error('Failed to load room members:', err);
    } finally {
      setIsLoadingMembers(false);
    }
  }, [roomId]);

  useEffect(() => {
    if (isOpen) {
      loadMembers();
    }
  }, [isOpen, loadMembers]);

  if (!isOpen) return null;

  const handleCopyDirectLink = async () => {
    const url = UrlUtils.getShareUrl(roomId);
    await UrlUtils.copyToClipboard(url);
    setCopiedDirect(true);
    setTimeout(() => setCopiedDirect(false), 2000);
  };

  const handleGenerateInvite = async () => {
    try {
      setIsGenerating(true);
      const invite = await apiService.createInvite(roomId, inviteRole);
      setGeneratedInvite(invite);
    } catch (err) {
      console.error('Failed to generate invite:', err);
    } finally {
      setIsGenerating(false);
    }
  };

  const getInviteUrl = (token: string) => {
    const protocol = window.location.protocol;
    const host = window.location.host;
    return `${protocol}//${host}/?invite=${token}`;
  };

  const handleCopyInviteLink = async () => {
    if (!generatedInvite) return;
    const url = getInviteUrl(generatedInvite.token);
    await UrlUtils.copyToClipboard(url);
    setCopiedInvite(true);
    setTimeout(() => setCopiedInvite(false), 2000);
  };

  const handleRoleChange = async (userUid: string, newRole: Role) => {
    try {
      await apiService.updateMemberRole(roomId, userUid, newRole);
      setMembers(prev =>
        prev.map(m => (m.user_uid === userUid ? { ...m, role: newRole } : m))
      );
    } catch (err) {
      console.error('Failed to update member role:', err);
    }
  };

  const handleRemoveMember = async (userUid: string) => {
    try {
      await apiService.removeMember(roomId, userUid);
      setMembers(prev => prev.filter(m => m.user_uid !== userUid));
    } catch (err) {
      console.error('Failed to remove member:', err);
    }
  };

  const isOwner = currentUserRole === 'owner';

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content share-modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Share "{roomName}"</h3>
          <button className="close-btn" onClick={onClose}>✕</button>
        </div>

        {/* Section 1: Direct Room Link */}
        <div className="share-section">
          <label className="section-label">Direct Document Link</label>
          <div className="copy-input-group">
            <input
              type="text"
              readOnly
              value={UrlUtils.getShareUrl(roomId)}
              className="share-url-input"
            />
            <button className="btn btn-secondary" onClick={handleCopyDirectLink}>
              {copiedDirect ? '✓ Copied' : 'Copy Link'}
            </button>
          </div>
        </div>

        {/* Section 2: Create Role-Specific Invite */}
        <div className="share-section">
          <label className="section-label">Create Invite Link with Role</label>
          <div className="invite-builder">
            <select
              value={inviteRole}
              onChange={(e) => setInviteRole(e.target.value as Role)}
              className="role-select"
            >
              <option value="editor">Editor (Can edit document)</option>
              <option value="viewer">Viewer (Read-only)</option>
            </select>
            <button
              className="btn btn-primary"
              onClick={handleGenerateInvite}
              disabled={isGenerating}
            >
              {isGenerating ? 'Generating...' : 'Create Invite Link'}
            </button>
          </div>

          {generatedInvite && (
            <div className="generated-invite-box">
              <span className="invite-badge">Role: {generatedInvite.role}</span>
              <div className="copy-input-group">
                <input
                  type="text"
                  readOnly
                  value={getInviteUrl(generatedInvite.token)}
                  className="share-url-input"
                />
                <button className="btn btn-secondary" onClick={handleCopyInviteLink}>
                  {copiedInvite ? '✓ Copied' : 'Copy'}
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Section 3: Document Members & Permissions */}
        <div className="share-section">
          <label className="section-label">Document Collaborators ({members.length})</label>
          {isLoadingMembers ? (
            <div className="members-loading">Loading collaborators...</div>
          ) : members.length === 0 ? (
            <div className="members-empty">No registered members yet. Invite someone above!</div>
          ) : (
            <div className="members-list">
              {members.map((member) => (
                <div key={member.id} className="member-row">
                  <div className="member-info">
                    <span className="member-name">{member.user_name || 'Anonymous User'}</span>
                    <span className="member-email">{member.user_email}</span>
                  </div>

                  <div className="member-actions">
                    {isOwner && member.role !== 'owner' ? (
                      <>
                        <select
                          value={member.role}
                          onChange={(e) => handleRoleChange(member.user_uid, e.target.value as Role)}
                          className="member-role-select"
                        >
                          <option value="editor">Editor</option>
                          <option value="viewer">Viewer</option>
                        </select>
                        <button
                          className="member-remove-btn"
                          title="Remove member"
                          onClick={() => handleRemoveMember(member.user_uid)}
                        >
                          ✕
                        </button>
                      </>
                    ) : (
                      <span className={`member-role-badge role-${member.role}`}>
                        {member.role}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default ShareModal;
