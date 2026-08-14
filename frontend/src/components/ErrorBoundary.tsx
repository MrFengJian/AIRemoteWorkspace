import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle } from "lucide-react";

import { Button } from "@/components/ui/button";

interface ErrorBoundaryProps {
  /** Shown in the crash card so the user knows which area failed. */
  label: string;
  /** Reset button label (localised by the caller). */
  resetLabel: string;
  children: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/**
 * Per-view error boundary. A render crash in one feature (hosts, terminal,
 * sftp, …) shows a recoverable error card instead of blanking the whole app —
 * important here because views stay mounted and share one window. Labels are
 * passed in because class components can't use the i18n hook.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`[${this.props.label}] render crashed:`, error, info.componentStack);
  }

  private reset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
          <AlertTriangle className="h-8 w-8 text-warning" />
          <div>
            <h2 className="text-lg font-medium text-foreground">
              {this.props.label}
            </h2>
            <p className="mt-1 max-w-md break-words font-mono text-xs text-muted-foreground">
              {this.state.error.message || String(this.state.error)}
            </p>
          </div>
          <Button variant="outline" onClick={this.reset}>
            {this.props.resetLabel}
          </Button>
        </div>
      );
    }
    return this.props.children;
  }
}
