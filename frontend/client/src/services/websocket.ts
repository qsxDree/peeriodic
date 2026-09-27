import { CursorPosition, Operation, WebSocketMessage } from '../types';
import apiService from './api';

export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

type MessageHandler = (message: WebSocketMessage) => void;
type StatusHandler = (status: ConnectionStatus) => void;
type ErrorHandler = (error: Event) => void;

interface ConnectOptions {
  name?: string;
  uid?: string;
}

class WebSocketService {
  private socket: WebSocket | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private baseReconnectDelay = 1000;
  private messageHandlers: MessageHandler[] = [];
  private statusHandlers: StatusHandler[] = [];
  private errorHandlers: ErrorHandler[] = [];
  private currentRoomId: string | null = null;
  private currentOptions: ConnectOptions | null = null;
  private status: ConnectionStatus = 'disconnected';
  private pingInterval: NodeJS.Timeout | null = null;

  getStatus(): ConnectionStatus {
    return this.status;
  }

  private setStatus(newStatus: ConnectionStatus) {
    this.status = newStatus;
    this.statusHandlers.forEach(handler => handler(newStatus));
  }

  connect(roomId: string, options?: ConnectOptions): Promise<void> {
    this.currentRoomId = roomId;
    if (options) {
      this.currentOptions = options;
    }

    return new Promise((resolve, reject) => {
      try {
        if (this.socket) {
          this.socket.close();
          this.socket = null;
        }

        this.setStatus(this.reconnectAttempts > 0 ? 'reconnecting' : 'connecting');

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const host = window.location.hostname;
        const port = '5000';
        let wsUrl = `${protocol}//${host}:${port}/ws?room=${encodeURIComponent(roomId)}`;

        if (this.currentOptions?.name) {
          wsUrl += `&name=${encodeURIComponent(this.currentOptions.name)}`;
        }
        if (this.currentOptions?.uid) {
          wsUrl += `&uid=${encodeURIComponent(this.currentOptions.uid)}`;
        }

        const token = apiService.getAuthToken();
        if (token) {
          wsUrl += `&token=${encodeURIComponent(token)}`;
        }

        console.log('🔌 Connecting to WebSocket:', wsUrl);
        this.socket = new WebSocket(wsUrl);

        this.socket.onopen = () => {
          console.log('✅ WebSocket connected to room:', roomId);
          this.reconnectAttempts = 0;
          this.setStatus('connected');
          this.startHeartbeat();
          resolve();
        };

        this.socket.onmessage = (event) => {
          try {
            const message: WebSocketMessage = JSON.parse(event.data);
            this.messageHandlers.forEach(handler => handler(message));
          } catch (error) {
            console.error('❌ Failed to parse WebSocket message:', error);
          }
        };

        this.socket.onclose = (event) => {
          console.log('❌ WebSocket closed:', event.code, event.reason);
          this.stopHeartbeat();
          if (this.currentRoomId === roomId) {
            this.handleReconnect(roomId);
          } else {
            this.setStatus('disconnected');
          }
        };

        this.socket.onerror = (error) => {
          console.error('⚠️ WebSocket error:', error);
          this.errorHandlers.forEach(handler => handler(error));
          reject(error);
        };
      } catch (error) {
        this.setStatus('disconnected');
        reject(error);
      }
    });
  }

  private handleReconnect(roomId: string) {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      this.setStatus('reconnecting');
      const delay = Math.min(this.baseReconnectDelay * Math.pow(1.5, this.reconnectAttempts - 1), 10000);
      console.log(`🔄 Attempting to reconnect (${this.reconnectAttempts}/${this.maxReconnectAttempts}) in ${delay}ms...`);

      setTimeout(() => {
        if (this.currentRoomId === roomId) {
          this.connect(roomId, this.currentOptions || undefined).catch(error => {
            console.error('❌ Reconnection failed:', error);
          });
        }
      }, delay);
    } else {
      console.error('❌ Max reconnection attempts reached');
      this.setStatus('disconnected');
    }
  }

  private startHeartbeat() {
    this.stopHeartbeat();
    this.pingInterval = setInterval(() => {
      this.send({ type: 'ping' });
    }, 25000);
  }

  private stopHeartbeat() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }

  disconnect() {
    this.stopHeartbeat();
    if (this.socket) {
      this.socket.close();
      this.socket = null;
      this.currentRoomId = null;
      this.currentOptions = null;
      this.setStatus('disconnected');
    }
  }

  send(message: WebSocketMessage) {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(message));
    } else {
      console.warn('⚠️ Cannot send message: WebSocket is not open', message);
    }
  }

  sendOp(rev: number, ops: Operation[]) {
    this.send({
      type: 'op',
      rev,
      ops,
    });
  }

  sendCursor(cursor: CursorPosition) {
    this.send({
      type: 'cursor',
      cursor,
    });
  }

  sendPresence(name?: string, color?: string) {
    this.send({
      type: 'presence',
      name,
      color,
    });
  }

  sendSync(rev: number) {
    this.send({
      type: 'sync',
      rev,
    });
  }

  onMessage(handler: MessageHandler) {
    this.messageHandlers.push(handler);
  }

  onStatusChange(handler: StatusHandler) {
    this.statusHandlers.push(handler);
    handler(this.status);
  }

  onConnect(handler: () => void) {
    this.statusHandlers.push(status => {
      if (status === 'connected') handler();
    });
  }

  onError(handler: ErrorHandler) {
    this.errorHandlers.push(handler);
  }

  isConnected(): boolean {
    return this.socket?.readyState === WebSocket.OPEN;
  }

  clearHandlers() {
    this.messageHandlers = [];
    this.statusHandlers = [];
    this.errorHandlers = [];
  }
}

const wsService = new WebSocketService();
export default wsService;
