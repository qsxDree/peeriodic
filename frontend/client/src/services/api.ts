import { 
  ApiResponse, 
  Room, 
  CreateRoomRequest, 
  CreateRoomResponse,
  SaveDocumentRequest,
  SaveDocumentResponse,
  User,
  RegisterRequest,
  LoginRequest,
  AuthResponse,
  Role,
  DocumentMember,
  RoomInvite
} from '../types';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:5000';
const TOKEN_STORAGE_KEY = 'peeriodic_auth_token';
const USER_STORAGE_KEY = 'peeriodic_auth_user';

class ApiService {
  private baseUrl: string;
  private token: string | null = null;
  private currentUser: User | null = null;

  constructor(baseUrl: string = API_BASE_URL) {
    this.baseUrl = baseUrl;
    this.token = localStorage.getItem(TOKEN_STORAGE_KEY);
    const savedUser = localStorage.getItem(USER_STORAGE_KEY);
    if (savedUser) {
      try {
        this.currentUser = JSON.parse(savedUser);
      } catch (e) {
        this.currentUser = null;
      }
    }
  }

  getAuthToken(): string | null {
    return this.token;
  }

  getCurrentUser(): User | null {
    return this.currentUser;
  }

  isAuthenticated(): boolean {
    return !!this.token;
  }

  private setSession(token: string, user: User) {
    this.token = token;
    this.currentUser = user;
    localStorage.setItem(TOKEN_STORAGE_KEY, token);
    localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user));
  }

  logout() {
    this.token = null;
    this.currentUser = null;
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    localStorage.removeItem(USER_STORAGE_KEY);
  }

  private async request<T>(
    endpoint: string, 
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string> || {}),
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    try {
      const response = await fetch(url, {
        ...options,
        headers,
      });
      
      if (!response.ok) {
        let errorMsg = `HTTP error! status: ${response.status}`;
        try {
          const errData = await response.json();
          if (errData && errData.error) {
            errorMsg = errData.error;
          }
        } catch (_) {}
        throw new Error(errorMsg);
      }

      return await response.json();
    } catch (error) {
      console.error('API request failed:', error);
      throw error;
    }
  }

  // --- Auth API ---
  async register(req: RegisterRequest): Promise<AuthResponse> {
    const response = await this.request<ApiResponse<AuthResponse>>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify(req),
    });
    const authData = response.data!;
    this.setSession(authData.token, authData.user);
    return authData;
  }

  async login(req: LoginRequest): Promise<AuthResponse> {
    const response = await this.request<ApiResponse<AuthResponse>>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(req),
    });
    const authData = response.data!;
    this.setSession(authData.token, authData.user);
    return authData;
  }

  async getMe(): Promise<User> {
    const response = await this.request<ApiResponse<User>>('/api/auth/me');
    const user = response.data!;
    this.currentUser = user;
    localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(user));
    return user;
  }

  // --- Room API methods ---
  async getRooms(uid?: string): Promise<Room[]> {
    const query = uid ? `?uid=${encodeURIComponent(uid)}` : '';
    const response = await this.request<ApiResponse<Room[]>>(`/api/rooms${query}`);
    return response.data || [];
  }

  async createRoom(request: CreateRoomRequest): Promise<CreateRoomResponse> {
    const response = await this.request<ApiResponse<CreateRoomResponse>>('/api/rooms', {
      method: 'POST',
      body: JSON.stringify(request),
    });
    return response.data!;
  }

  async getRoom(roomId: string): Promise<Room> {
    const response = await this.request<ApiResponse<Room>>(`/api/rooms/${roomId}`);
    return response.data!;
  }

  async deleteRoom(roomId: string): Promise<void> {
    await this.request(`/api/rooms/${roomId}`, {
      method: 'DELETE',
    });
  }

  // --- Sharing & Invites API methods ---
  async createInvite(roomId: string, role: Role, expiresInHours?: number): Promise<RoomInvite> {
    const response = await this.request<ApiResponse<RoomInvite>>(`/api/rooms/${roomId}/invites`, {
      method: 'POST',
      body: JSON.stringify({ role, expires_in_hours: expiresInHours }),
    });
    return response.data!;
  }

  async getRoomMembers(roomId: string): Promise<DocumentMember[]> {
    const response = await this.request<ApiResponse<DocumentMember[]>>(`/api/rooms/${roomId}/members`);
    return response.data || [];
  }

  async updateMemberRole(roomId: string, userUid: string, role: Role): Promise<void> {
    await this.request(`/api/rooms/${roomId}/members/${userUid}`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    });
  }

  async removeMember(roomId: string, userUid: string): Promise<void> {
    await this.request(`/api/rooms/${roomId}/members/${userUid}`, {
      method: 'DELETE',
    });
  }

  async acceptInvite(token: string): Promise<{ room: Room; role: Role }> {
    const response = await this.request<ApiResponse<{ room: Room; role: Role }>>(`/api/invites/${token}/accept`, {
      method: 'POST',
    });
    return response.data!;
  }

  // --- Document API methods ---
  async saveDocument(roomId: string, content: string): Promise<SaveDocumentResponse> {
    const request: SaveDocumentRequest = { content };
    const response = await this.request<ApiResponse<SaveDocumentResponse>>(`/api/save?room=${roomId}`, {
      method: 'POST',
      body: JSON.stringify(request),
    });
    return response.data!;
  }

  async getDocument(roomId: string): Promise<Room> {
    return this.getRoom(roomId);
  }
}

// Export singleton instance
export const apiService = new ApiService();
export default apiService;
