import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { ChatConnectionContext, useChatConnected } from "./ChatRealtimeContext";

afterEach(cleanup);

function Probe() {
  const connected = useChatConnected();
  return <span data-testid="value">{String(connected)}</span>;
}

describe("useChatConnected", () => {
  it("returns the value provided by ChatConnectionContext.Provider", () => {
    render(
      <ChatConnectionContext.Provider value={true}>
        <Probe />
      </ChatConnectionContext.Provider>,
    );
    expect(screen.getByTestId("value").textContent).toBe("true");
  });

  it("defaults to false when no provider overrides the context", () => {
    render(<Probe />);
    expect(screen.getByTestId("value").textContent).toBe("false");
  });
});
