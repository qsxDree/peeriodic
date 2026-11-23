import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import Dashboard from './Dashboard';

describe('Dashboard Component', () => {
  const mockOpenRoom = jest.fn();
  const mockOpenAuth = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders dashboard heading, search bar, and create button for guest', () => {
    render(
      <Dashboard
        currentUser={null}
        onOpenRoom={mockOpenRoom}
        onOpenAuth={mockOpenAuth}
      />
    );

    expect(screen.getByRole('heading', { name: /your documents/i })).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/search documents by title/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /create new room/i })).toBeInTheDocument();
    expect(screen.getByText(/you are using peeriodic as a guest/i)).toBeInTheDocument();
  });

  test('renders personalized welcome for authenticated user', () => {
    const user = {
      uid: 'user-1',
      name: 'Alice Johnson',
      email: 'alice@example.com',
    };

    render(
      <Dashboard
        currentUser={user}
        onOpenRoom={mockOpenRoom}
        onOpenAuth={mockOpenAuth}
      />
    );

    expect(screen.getByText(/welcome back, alice johnson!/i)).toBeInTheDocument();
    expect(screen.queryByText(/you are using peeriodic as a guest/i)).not.toBeInTheDocument();
  });

  test('opens create document modal when clicking Create New Room', () => {
    render(
      <Dashboard
        currentUser={null}
        onOpenRoom={mockOpenRoom}
        onOpenAuth={mockOpenAuth}
      />
    );

    const createBtn = screen.getByRole('button', { name: /create new room/i });
    fireEvent.click(createBtn);

    expect(screen.getByRole('heading', { name: /create new document/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/document title/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/default link access/i)).toBeInTheDocument();
  });

  test('allows direct room joining via input', () => {
    render(
      <Dashboard
        currentUser={null}
        onOpenRoom={mockOpenRoom}
        onOpenAuth={mockOpenAuth}
      />
    );

    const input = screen.getByPlaceholderText(/enter room id or link/i);
    const joinBtn = screen.getByRole('button', { name: /join room/i });

    fireEvent.change(input, { target: { value: 'custom-room-123' } });
    fireEvent.click(joinBtn);

    expect(mockOpenRoom).toHaveBeenCalledWith('custom-room-123', 'Shared Document');
  });
});
