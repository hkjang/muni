import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { HandoffSettings } from "./HandoffSettings";

afterEach(cleanup);

describe("HandoffSettings", () => {
  it("starts empty and says so", () => {
    render(<HandoffSettings value={{ peers: [] }} onChange={vi.fn()} />);
    expect(screen.getByText(/아직 없습니다/)).toBeTruthy();
  });

  it("adds a peer that receives markdown by default", () => {
    const onChange = vi.fn();
    render(<HandoffSettings value={{ peers: [] }} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "서비스 더하기" }));
    expect(onChange).toHaveBeenCalledWith({
      peers: [{ origin: "", name: "", receives: ["markdown"] }],
    });
  });

  it("edits the address, toggles a format and removes a peer", () => {
    const onChange = vi.fn();
    const peers = [
      { origin: "https://ptium.intra", name: "ptium", receives: ["markdown"] },
    ];
    render(<HandoffSettings value={{ peers }} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("주소"), {
      target: { value: "https://weekly.intra" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      peers: [
        {
          origin: "https://weekly.intra",
          name: "ptium",
          receives: ["markdown"],
        },
      ],
    });
    fireEvent.click(screen.getByLabelText("DOCX"));
    expect(onChange).toHaveBeenLastCalledWith({
      peers: [
        {
          origin: "https://ptium.intra",
          name: "ptium",
          receives: ["markdown", "docx"],
        },
      ],
    });
    fireEvent.click(screen.getByLabelText("Markdown"));
    expect(onChange).toHaveBeenLastCalledWith({
      peers: [{ origin: "https://ptium.intra", name: "ptium", receives: [] }],
    });
    fireEvent.click(screen.getByRole("button", { name: "서비스 지우기" }));
    expect(onChange).toHaveBeenLastCalledWith({ peers: [] });
  });
});
