// API Response types
export interface ApiResponse<T = any> {
  success: boolean;
  message?: string;
  data?: T;
  error?: string;
}

// User and Auth types
export type Role = 'owner' | 'editor' | 'viewer';

export interface User {
  id?: number;
  uid: string;
  email: string;
  name: string;
  created_at?: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  name?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}

// Room types
export interface Room {
  id: string;
  title: string;
  content?: string;
  current_rev?: number;
  role?: Role;
  public_role?: Role;
  user_uid?: string;
  created_at?: string;
}

export interface CreateRoomRequest {
  title: string;
  uid?: string;
}

export interface CreateRoomResponse {
  id: string;
  title: string;
}

// Sharing & Member types
export interface DocumentMember {
  id: number;
  room_id: string;
  user_uid: string;
  role: Role;
  user_name?: string;
  user_email?: string;
  created_at: string;
  updated_at?: string;
}

export interface RoomInvite {
  id: number;
  room_id: string;
  token: string;
  role: Role;
  created_by?: string;
  expires_at?: string;
  created_at: string;
}

// Document types
export interface SaveDocumentRequest {
  content: string;
}

export interface SaveDocumentResponse {
  status: string;
  roomId: string;
  contentLength: number;
}

// OT Operation types
export type OpType = 'insert' | 'delete';

export interface Operation {
  type: OpType;
  pos: number;
  text?: string;
  len?: number;
}

// Cursor and Presence types
export interface CursorPosition {
  pos: number;
  sel_start: number;
  sel_end: number;
}

export interface UserPresence {
  client_id: string;
  uid?: string;
  name: string;
  color: string;
  role?: Role;
  cursor?: CursorPosition;
  connected_at?: string;
}

// WebSocket message types
export type WebSocketMessageType =
  | 'init'
  | 'op'
  | 'ack'
  | 'cursor'
  | 'presence'
  | 'sync'
  | 'ping'
  | 'pong'
  | 'error'
  | 'update';

export interface WebSocketMessage {
  type: WebSocketMessageType;
  rev?: number;
  data?: string;
  client_id?: string;
  role?: Role;
  ops?: Operation[];
  presence?: UserPresence[];
  cursor?: CursorPosition;
  name?: string;
  color?: string;
}

// Component props types
export interface RoomListProps {
  rooms: Room[];
  onRoomClick: (roomId: string) => void;
  onRoomRename: (roomId: string, newName: string) => void;
  onRoomDelete: (roomId: string) => void;
  onRoomShare: (roomId: string) => void;
}

export interface EditorProps {
  roomId: string;
  roomName: string;
  onBack: () => void;
  onShare: () => void;
}

export interface HomeProps {
  onCreateRoom: () => void;
  onRoomClick: (roomId: string) => void;
}
