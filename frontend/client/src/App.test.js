import { render, screen } from '@testing-library/react';
import App from './App';

test('renders Peeriodic title and Create New Room button', () => {
  render(<App />);
  expect(screen.getByRole('heading', { name: /peeriodic/i })).toBeInTheDocument();
  expect(screen.getByText(/Create and share documents in real-time/i)).toBeInTheDocument();
  expect(screen.getByText('Create New Room')).toBeInTheDocument();
  expect(screen.getByText(/Sign In/i)).toBeInTheDocument();
});

