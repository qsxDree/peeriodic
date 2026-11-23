import { render, screen } from '@testing-library/react';

jest.mock('./components/CodeMirrorEditor', () => {
  return function MockCodeMirrorEditor() {
    return <div data-testid="mock-codemirror-editor">CodeMirror Editor</div>;
  };
});

import App from './App';

test('renders Peeriodic brand, Your Documents heading, and Create New Room button', () => {
  render(<App />);
  expect(screen.getByText('Peeriodic')).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: /your documents/i })).toBeInTheDocument();
  expect(screen.getByText(/Create and share documents in real-time/i)).toBeInTheDocument();
  expect(screen.getByText('Create New Room')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /sign in \/ register/i })).toBeInTheDocument();
});

