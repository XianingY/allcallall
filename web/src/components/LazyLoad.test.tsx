import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { LazyLoad } from './LazyLoad';
import '@testing-library/jest-dom/vitest';

describe('LazyLoad', () => {
  it('shows loading state while component loads', async () => {
    const loader = () => new Promise<{ default: React.ComponentType<any> }>(resolve => {
      setTimeout(() => resolve({ default: () => <div data-testid="slow-component">Loaded!</div> }), 100);
    });

    render(
      <LazyLoad
        loader={loader}
        fallback={<div data-testid="loading">Loading...</div>}
      />
    );
    
    expect(screen.getByTestId('loading')).toBeInTheDocument();
    
    await waitFor(() => {
      expect(screen.getByTestId('slow-component')).toBeInTheDocument();
    });
  });

  it('shows error state on load failure', async () => {
    const failingLoader = () => Promise.reject(new Error('Load failed'));
    
    // Suppress console.error for this test as the ErrorBoundary will log it
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    render(
      <LazyLoad
        loader={failingLoader}
        fallback={<div>Loading...</div>}
        errorFallback={<div data-testid="error">Error occurred</div>}
      />
    );
    
    await waitFor(() => {
      expect(screen.getByTestId('error')).toBeInTheDocument();
    });

    consoleSpy.mockRestore();
  });
});
