import { create } from 'zustand';
import { UserPresence, CursorPosition, Role } from '../types';
import { ConnectionStatus } from '../services/websocket';

export interface RemoteCursor {
  name: string;
  color: string;
  cursor: CursorPosition;
}

interface DocState {
  // Document information
  roomId: string;
  title: string;
  content: string;
  rev: number;
  role: Role;
  isLoading: boolean;
  isSaving: boolean;

  // Real-time connection
  connectionStatus: ConnectionStatus;
  reconnectAttempts: number;
  pendingOpsCount: number;
  lastSyncedAt: Date | null;
  offlineBannerDismissed: boolean;

  // User presence & session
  myClientId: string;
  myName: string;
  myColor: string;
  presences: UserPresence[];
  remoteCursors: Record<string, RemoteCursor>;

  // Actions
  initDocument: (roomId: string, title: string, content: string, rev: number, role?: Role) => void;
  setContent: (content: string, rev?: number) => void;
  setTitle: (title: string) => void;
  setRole: (role: Role) => void;
  setIsLoading: (isLoading: boolean) => void;
  setIsSaving: (isSaving: boolean) => void;

  setConnectionStatus: (status: ConnectionStatus, attempts?: number) => void;
  setPendingOpsCount: (count: number) => void;
  setLastSyncedAt: (date: Date) => void;
  dismissOfflineBanner: () => void;

  setMySession: (clientId: string, name: string, color: string, role?: Role) => void;
  setPresences: (presences: UserPresence[]) => void;
  updateRemoteCursor: (clientId: string, name: string, color: string, cursor: CursorPosition) => void;
  removeRemoteCursor: (clientId: string) => void;

  reset: () => void;
}

const initialState = {
  roomId: '',
  title: 'Untitled Document',
  content: '',
  rev: 0,
  role: 'editor' as Role,
  isLoading: true,
  isSaving: false,

  connectionStatus: 'disconnected' as ConnectionStatus,
  reconnectAttempts: 0,
  pendingOpsCount: 0,
  lastSyncedAt: null,
  offlineBannerDismissed: false,

  myClientId: '',
  myName: 'Anonymous',
  myColor: '#007bff',
  presences: [] as UserPresence[],
  remoteCursors: {} as Record<string, RemoteCursor>,
};

export const useDocStore = create<DocState>((set) => ({
  ...initialState,

  initDocument: (roomId, title, content, rev, role = 'editor') =>
    set({
      roomId,
      title: title || 'Untitled Document',
      content,
      rev,
      role,
      isLoading: false,
      lastSyncedAt: new Date(),
    }),

  setContent: (content, rev) =>
    set((state) => ({
      content,
      rev: rev !== undefined ? rev : state.rev,
    })),

  setTitle: (title) => set({ title }),

  setRole: (role) => set({ role }),

  setIsLoading: (isLoading) => set({ isLoading }),

  setIsSaving: (isSaving) => set({ isSaving }),

  setConnectionStatus: (connectionStatus, attempts = 0) =>
    set((state) => ({
      connectionStatus,
      reconnectAttempts: attempts,
      offlineBannerDismissed: connectionStatus === 'connected' ? false : state.offlineBannerDismissed,
      lastSyncedAt: connectionStatus === 'connected' ? new Date() : state.lastSyncedAt,
    })),

  setPendingOpsCount: (pendingOpsCount) => set({ pendingOpsCount }),

  setLastSyncedAt: (lastSyncedAt) => set({ lastSyncedAt }),

  dismissOfflineBanner: () => set({ offlineBannerDismissed: true }),

  setMySession: (myClientId, myName, myColor, role) =>
    set((state) => ({
      myClientId,
      myName,
      myColor,
      role: role || state.role,
    })),

  setPresences: (presences) => set({ presences }),

  updateRemoteCursor: (clientId, name, color, cursor) =>
    set((state) => ({
      remoteCursors: {
        ...state.remoteCursors,
        [clientId]: { name, color, cursor },
      },
    })),

  removeRemoteCursor: (clientId) =>
    set((state) => {
      const next = { ...state.remoteCursors };
      delete next[clientId];
      return { remoteCursors: next };
    }),

  reset: () => set(initialState),
}));
