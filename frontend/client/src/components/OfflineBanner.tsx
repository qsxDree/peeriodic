import React, { useState, useEffect } from 'react';
import { useDocStore } from '../store/docStore';
import wsService from '../services/websocket';
import './OfflineBanner.css';

export const OfflineBanner: React.FC = () => {
  const connectionStatus = useDocStore((s) => s.connectionStatus);
  const reconnectAttempts = useDocStore((s) => s.reconnectAttempts);
  const pendingOpsCount = useDocStore((s) => s.pendingOpsCount);
  const offlineBannerDismissed = useDocStore((s) => s.offlineBannerDismissed);
  const dismissOfflineBanner = useDocStore((s) => s.dismissOfflineBanner);

  const [showReconnectedToast, setShowReconnectedToast] = useState(false);
  const [prevStatus, setPrevStatus] = useState(connectionStatus);

  useEffect(() => {
    // If transitioning from disconnected/reconnecting to connected, show success toast
    if ((prevStatus === 'disconnected' || prevStatus === 'reconnecting') && connectionStatus === 'connected') {
      setShowReconnectedToast(true);
      const timer = setTimeout(() => setShowReconnectedToast(false), 3500);
      return () => clearTimeout(timer);
    }
    setPrevStatus(connectionStatus);
  }, [connectionStatus, prevStatus]);

  const handleManualRetry = () => {
    wsService.reconnect();
  };

  if (showReconnectedToast) {
    return (
      <div className="status-toast status-toast-success animate-fade-in">
        <span className="toast-icon">✅</span>
        <span className="toast-text">Back online! Document successfully synchronized.</span>
      </div>
    );
  }

  if (connectionStatus === 'connected' || offlineBannerDismissed) {
    return null;
  }

  const isReconnecting = connectionStatus === 'reconnecting';

  return (
    <div className={`offline-banner ${isReconnecting ? 'offline-banner-reconnecting' : 'offline-banner-disconnected'} animate-slide-down`}>
      <div className="offline-banner-content">
        <span className="banner-icon">{isReconnecting ? '🔄' : '⚠️'}</span>
        <div className="banner-text-group">
          <strong>
            {isReconnecting
              ? `Connection lost. Reconnecting... (Attempt ${reconnectAttempts})`
              : 'You are currently working offline.'}
          </strong>
          <span className="banner-subtext">
            {pendingOpsCount > 0
              ? ` ${pendingOpsCount} unsaved edit${pendingOpsCount > 1 ? 's' : ''} queued locally and will sync once reconnected.`
              : ' You can continue editing; your changes will be queued.'}
          </span>
        </div>
      </div>

      <div className="offline-banner-actions">
        <button className="btn-retry" onClick={handleManualRetry} title="Attempt reconnection immediately">
          Retry Now
        </button>
        <button className="btn-dismiss" onClick={dismissOfflineBanner} title="Dismiss notification" aria-label="Dismiss notification">
          ✕
        </button>
      </div>
    </div>
  );
};

export default OfflineBanner;
