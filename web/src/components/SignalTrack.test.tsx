import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SignalTrack } from "@/components/SignalTrack";

describe("SignalTrack", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(cleanup);

  it("exposes realtime state politely", () => {
    render(
      <SignalTrack state="connected" activity="agent" label="Agent 正在整理资料" />,
    );

    const track = screen.getByRole("status");
    expect(track).toHaveAttribute("aria-live", "polite");
    expect(track).toHaveTextContent("Agent 正在整理资料");
    expect(track.className).toContain("signal-track-active");
  });

  it("keeps connecting and compact states quiet", () => {
    const { rerender } = render(
      <SignalTrack state="connecting" label="正在连接" />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("正在连接");
    expect(screen.getByRole("status").className).not.toContain("signal-track-active");

    rerender(<SignalTrack state="connected" label="实时在线" compact />);
    expect(screen.getByRole("status").className).toContain("signal-track-compact");
  });
});
