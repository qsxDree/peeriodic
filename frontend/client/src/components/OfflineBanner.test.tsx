import React from 'react';
import { render, screen, fireEvent, act } from '@testing-library/react';
import OfflineBanner from './OfflineBanner';
import { useDocStore } from '../store/docStore';

describe('OfflineBanner', () => {
  beforeEach(() => {
    act(() => {
      useDocStore.getState().reset();
    });
  });

  test('does not render when connected', () => {
    act(() => {
      useDocStore.getState().setConnectionStatus('connected');
    });

    render(<OfflineBanner />);
    expect(screen.queryByText(/offline/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/reconnecting/i)).not.toBeInTheDocument();
  });

  test('renders reconnecting message with attempt count', () => {
    act(() => {
      useDocStore.getState().setConnectionStatus('reconnecting', 3);
      useDocStore.getState().setPendingOpsCount(2);
    });

    render(<OfflineBanner />);
    expect(screen.getByText(/reconnecting... \(attempt 3\)/i)).toBeInTheDocument();
    expect(screen.getByText(/2 unsaved edits queued locally/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /retry now/i })).toBeInTheDocument();
  });

  test('renders offline message when disconnected', () => {
    act(() => {
      useDocStore.getState().setConnectionStatus('disconnected');
    });

    render(<OfflineBanner />);
    expect(screen.getByText(/you are currently working offline/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /retry now/i })).toBeInTheDocument();
  });

  test('can be dismissed via close button', () => {
    act(() => {
      useDocStore.getState().setConnectionStatus('disconnected');
    });

    render(<OfflineBanner />);
    const closeBtn = screen.getByRole('button', { name: /dismiss notification/i });
    fireEvent.click(closeBtn);

    expect(screen.queryByText(/you are currently working offline/i)).not.toBeInTheDocument();
  });
});
