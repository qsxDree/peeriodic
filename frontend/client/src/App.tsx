import React, { useState, useEffect, useCallback } from 'react';
import { StorageService } from './utils/storage';
import { UrlUtils } from './utils/url';
import apiService from './services/api';
import { User } from './types';
import Dashboard from './components/Dashboard';
import Editor from './components/Editor';
import AuthModal from './components/AuthModal';
import './App.css';

const App: React.FC = () => {
  const [currentRoomId, setCurrentRoomId] = useState<string | null>(null);
  const [roomName, setRoomName] = useState('Shared Room');
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);

  useEffect(() => {
    // Check authentication status on startup
    if (apiService.isAuthenticated()) {
      apiService.getMe()
        .then(setCurrentUser)
        .catch(() => {
          apiService.logout();
          setCurrentUser(null);
        });
    }

    // Check for invite token in URL (?invite=<token>)
    const inviteToken = new URLSearchParams(window.location.search).get('invite');
    if (inviteToken) {
      apiService.acceptInvite(inviteToken)
        .then(({ room }) => {
          UrlUtils.setRoomIdInUrl(room.id);
          setCurrentRoomId(room.id);
          setRoomName(room.title);
        })
        .catch((err) => {
          console.error('Failed to accept invite:', err);
        });
    } else {
      // Check if we're in a room from URL
      const roomId = UrlUtils.getRoomIdFromUrl();
      if (roomId) {
        setCurrentRoomId(roomId);
        const savedRooms = StorageService.getRooms();
        const matchedRoom = savedRooms.find(room => room.id === roomId);
        setRoomName(matchedRoom?.title || 'Shared Room');
      }
    }
  }, []);

  const handleBackToHome = useCallback(() => {
    UrlUtils.removeRoomIdFromUrl();
    setCurrentRoomId(null);
    setRoomName('Shared Room');
  }, []);

  const handleShare = useCallback(() => {
    console.log('Share action triggered');
  }, []);

  const handleLogout = useCallback(() => {
    apiService.logout();
    setCurrentUser(null);
  }, []);

  return (
    <div className="App-container">
      {/* Top Navbar */}
      <header className="top-nav">
        <div className="brand" onClick={handleBackToHome}>
          <span className="brand-icon">📝</span>
          <span className="brand-name">Peeriodic</span>
        </div>

        <div className="nav-actions">
          {currentUser ? (
            <div className="user-badge-group">
              <span className="user-name">👤 {currentUser.name}</span>
              <button onClick={handleLogout} className="btn btn-outline-sm">
                Sign Out
              </button>
            </div>
          ) : (
            <button onClick={() => setIsAuthModalOpen(true)} className="btn btn-primary-sm">
              Sign In / Register
            </button>
          )}
        </div>
      </header>

      {/* Main Content Area */}
      <main className="main-content">
        {!currentRoomId ? (
          <Dashboard
            currentUser={currentUser}
            onOpenRoom={(id, title) => {
              UrlUtils.setRoomIdInUrl(id);
              setCurrentRoomId(id);
              setRoomName(title);
            }}
            onOpenAuth={() => setIsAuthModalOpen(true)}
          />
        ) : (
          <Editor
            roomId={currentRoomId}
            roomName={roomName}
            onBack={handleBackToHome}
            onShare={handleShare}
          />
        )}
      </main>

      {/* Auth Modal */}
      <AuthModal
        isOpen={isAuthModalOpen}
        onClose={() => setIsAuthModalOpen(false)}
        onAuthSuccess={(user) => setCurrentUser(user)}
      />
    </div>
  );
};

export default App;
