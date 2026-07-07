import React, { Suspense, lazy, ComponentType, ReactNode } from 'react';

interface LazyLoadProps {
  loader: () => Promise<{ default: ComponentType<any> }>;
  fallback?: ReactNode;
  errorFallback?: ReactNode;
  onError?: (error: Error) => void;
}

interface LazyLoadState {
  hasError: boolean;
  error?: Error;
}

export class LazyLoad extends React.Component<LazyLoadProps, LazyLoadState> {
  constructor(props: LazyLoadProps) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): LazyLoadState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('LazyLoad error:', error, errorInfo);
    this.props.onError?.(error);
  }

  render() {
    if (this.state.hasError) {
      return this.props.errorFallback || (
        <div className="p-4 text-center">
          <p className="text-red-500 mb-2">Failed to load module.</p>
          <button 
            className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
            onClick={() => window.location.reload()}
          >
            Refresh Page
          </button>
        </div>
      );
    }

    const LazyComponent = lazy(this.props.loader);

    return (
      <Suspense fallback={this.props.fallback || <div>Loading...</div>}>
        <LazyComponent />
      </Suspense>
    );
  }
}
