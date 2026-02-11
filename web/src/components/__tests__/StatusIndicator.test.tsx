import React from 'react';
import { render, screen } from '@testing-library/react';
import StatusIndicator from '../StatusIndicator';

describe('StatusIndicator', () => {
  test('renders healthy status correctly', () => {
    render(<StatusIndicator status="healthy" />);
    
    const badge = screen.getByTestId('status-indicator');
    expect(badge).toBeInTheDocument();
  });

  test('renders warning status correctly', () => {
    render(<StatusIndicator status="warning" />);
    
    const badge = screen.getByTestId('status-indicator');
    expect(badge).toBeInTheDocument();
  });

  test('renders critical status correctly', () => {
    render(<StatusIndicator status="critical" />);
    
    const badge = screen.getByTestId('status-indicator');
    expect(badge).toBeInTheDocument();
  });

  test('shows text when showText is true', () => {
    render(<StatusIndicator status="healthy" showText={true} />);

    expect(screen.getByText('健康')).toBeInTheDocument();
  });

  test('does not show text when showText is false', () => {
    render(<StatusIndicator status="healthy" showText={false} />);

    expect(screen.queryByText('健康')).not.toBeInTheDocument();
  });
});
